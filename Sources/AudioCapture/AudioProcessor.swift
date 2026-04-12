@preconcurrency import AVFAudio
import CoreAudio

/// Bridges the real-time audio callback and the writer.
///
/// The IO callback runs on Apple's real-time audio thread — we do the
/// bare minimum there (copy data), then dispatch to a background queue
/// where we convert Float32 → Int16 and write to the output.
/// No sample rate conversion — we keep the native rate for quality.
///
/// Thread safety: The IO block only enqueues data onto processingQueue.
/// All mutable state (converter, writer) is accessed only from processingQueue.
final class AudioProcessor: @unchecked Sendable {
    private let inputFormat: AVAudioFormat
    private let outputFormat: AVAudioFormat
    private let converter: AVAudioConverter
    private let writer: AudioWriting
    private let processingQueue = DispatchQueue(label: "audio.processing", qos: .userInitiated)

    init(tapFormat: AudioStreamBasicDescription, writer: AudioWriting) throws {
        var tapFormatCopy = tapFormat
        guard let inFmt = AVAudioFormat(streamDescription: &tapFormatCopy) else {
            throw AudioCaptureError.formatError(
                "Could not create input format from tap (rate: \(tapFormat.mSampleRate), "
                + "channels: \(tapFormat.mChannelsPerFrame))"
            )
        }
        self.inputFormat = inFmt

        // Set the output sample rate to match the tap's native rate (usually 48kHz).
        // No resampling needed — we only convert Float32 → Int16.
        OutputFormat.sampleRate = inFmt.sampleRate

        guard let outFmt = AVAudioFormat(
            commonFormat: .pcmFormatInt16,
            sampleRate: inFmt.sampleRate,
            channels: AVAudioChannelCount(OutputFormat.channels),
            interleaved: true
        ) else {
            throw AudioCaptureError.formatError(
                "Could not create Int16 output format at \(Int(inFmt.sampleRate))Hz"
            )
        }
        self.outputFormat = outFmt

        guard let conv = AVAudioConverter(from: inFmt, to: outFmt) else {
            throw AudioCaptureError.formatError(
                "Could not create format converter at \(Int(inFmt.sampleRate))Hz"
            )
        }
        self.converter = conv
        self.writer = writer
    }

    /// Returns the IO block to pass to AudioTapManager.startStreaming().
    ///
    /// This closure runs on Apple's real-time audio thread. We copy the
    /// raw float samples into a Data object and dispatch it to our
    /// processing queue. That's it — no heavy work on the RT thread.
    func makeIOBlock() -> AudioDeviceIOBlock {
        return { [weak self] _, inInputData, _, _, _ in
            guard let self else { return }

            // inInputData is an AudioBufferList. For our mono tap, there's 1 buffer
            // containing Float32 samples at the native sample rate.
            let bufferList = inInputData.pointee
            guard bufferList.mNumberBuffers > 0 else { return }

            let buffer = bufferList.mBuffers
            guard let rawData = buffer.mData, buffer.mDataByteSize > 0 else { return }

            // Copy the bytes off the RT thread — this allocates, which is
            // technically a no-no on RT threads, but is fine for a capture tool.
            // If we ever see audio glitches, swap this for a lock-free ring buffer.
            let data = Data(bytes: rawData, count: Int(buffer.mDataByteSize))

            self.processingQueue.async { [weak self] in
                self?.processAudioData(data)
            }
        }
    }

    // MARK: - Private

    /// Convert audio from the tap's native format to our target format and write it out.
    private func processAudioData(_ data: Data) {
        // Wrap the raw Float32 data in an AVAudioPCMBuffer
        let bytesPerFrame = inputFormat.streamDescription.pointee.mBytesPerFrame
        let frameCount = AVAudioFrameCount(data.count) / bytesPerFrame

        guard frameCount > 0 else {
            fputs("Warning: received empty audio data (\(data.count) bytes)\n", stderr)
            return
        }

        guard let inputBuffer = AVAudioPCMBuffer(pcmFormat: inputFormat, frameCapacity: frameCount) else {
            fputs("Warning: could not create input buffer for \(frameCount) frames\n", stderr)
            return
        }

        inputBuffer.frameLength = frameCount

        // Copy our data into the input buffer
        data.withUnsafeBytes { rawBytes in
            guard let src = rawBytes.baseAddress else { return }
            if let dest = inputBuffer.floatChannelData?[0] {
                memcpy(dest, src, data.count)
            }
        }

        // No sample rate conversion — output has same frame count as input
        let outputFrameCount = frameCount

        guard let outputBuffer = AVAudioPCMBuffer(
            pcmFormat: outputFormat, frameCapacity: outputFrameCount
        ) else {
            fputs("Warning: could not create output buffer for \(outputFrameCount) frames\n", stderr)
            return
        }

        // AVAudioConverter uses a "pull" model — it calls our block to get input.
        // For each convert() call, we provide the input buffer exactly once.
        // nonisolated(unsafe) because the converter calls this block synchronously —
        // there's no actual concurrent access, but Swift 6 can't prove that.
        nonisolated(unsafe) var hasProvidedInput = false
        var conversionError: NSError?

        converter.convert(to: outputBuffer, error: &conversionError) { _, outStatus in
            if hasProvidedInput {
                outStatus.pointee = .noDataNow
                return nil
            }
            hasProvidedInput = true
            outStatus.pointee = .haveData
            return inputBuffer
        }

        if let error = conversionError {
            fputs("Conversion error: \(error.localizedDescription)\n", stderr)
            return
        }

        // Extract the Int16 samples from the output buffer and send to writer
        guard outputBuffer.frameLength > 0,
              let int16Data = outputBuffer.int16ChannelData
        else {
            fputs("Warning: converter produced no output frames\n", stderr)
            return
        }

        let byteCount = Int(outputBuffer.frameLength) * MemoryLayout<Int16>.size
        let pcmData = Data(bytes: int16Data[0], count: byteCount)

        do {
            try writer.write(pcmData)
        } catch {
            fputs("Write error: \(error.localizedDescription)\n", stderr)
        }
    }
}