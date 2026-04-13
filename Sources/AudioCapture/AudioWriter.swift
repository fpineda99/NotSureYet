import Foundation

// MARK: - Output Format Constants

/// The target audio format for the capture pipeline.
/// Sample rate is set at runtime from the system's native rate (usually 48kHz).
/// Channels and bit depth are fixed — mono Int16 is the standard for speech.
enum OutputFormat {
    nonisolated(unsafe) static var sampleRate: Double = 48000  // Set from tap format at startup
    static let channels: UInt32 = 1
    static let bitsPerSample: UInt32 = 16
    static let bytesPerSample: UInt32 = bitsPerSample / 8
}

// MARK: - Protocol

/// Anything that can receive PCM audio data and write it somewhere.
/// The lifecycle is: prepare() → write() repeatedly → finalize()
protocol AudioWriting: Sendable {
    func prepare() throws
    func write(_ data: Data) throws
    func finalize() throws
}

// MARK: - WAV File Writer

/// Writes mono 16-bit PCM audio to a WAV file at the system's native sample rate.
///
/// WAV format is simple: a 44-byte header describing the audio,
/// followed by raw PCM samples. We write the header with placeholder
/// sizes first, then update them in finalize() once we know the total.
final class WAVFileWriter: AudioWriting, @unchecked Sendable {
    private let path: String
    private var fileHandle: FileHandle?
    private var dataSize: UInt32 = 0

    init(path: String) {
        self.path = path
    }

    func prepare() throws {
        // Create the file (or overwrite if it exists)
        let fileManager = FileManager.default
        guard fileManager.createFile(atPath: path, contents: nil) else {
            throw AudioCaptureError.fileCreationFailed(path)
        }

        guard let handle = FileHandle(forWritingAtPath: path) else {
            throw AudioCaptureError.fileCreationFailed(path)
        }
        self.fileHandle = handle

        // Write WAV header with placeholder sizes (we'll update in finalize)
        let header = buildWAVHeader(dataSize: 0xFFFFFFFF)
        handle.write(header)
    }

    func write(_ data: Data) throws {
        guard let handle = fileHandle else {
            fputs("[AudioCapture] Warning: write called but file is not open\n", stderr)
            return
        }
        handle.write(data)
        dataSize += UInt32(data.count)
    }

    func finalize() throws {
        guard let handle = fileHandle else { return }

        // Update the WAV header with actual sizes
        // Byte 4: total file size - 8
        handle.seek(toFileOffset: 4)
        var fileSize = dataSize + 36  // 44 (header) - 8 (RIFF + size field)
        handle.write(Data(bytes: &fileSize, count: 4))

        // Byte 40: data chunk size
        handle.seek(toFileOffset: 40)
        var writtenDataSize = dataSize
        handle.write(Data(bytes: &writtenDataSize, count: 4))

        handle.closeFile()
        fileHandle = nil
    }

    /// Builds the 44-byte WAV header.
    ///
    /// Layout (all little-endian):
    ///   0-3:   "RIFF"
    ///   4-7:   file size - 8
    ///   8-11:  "WAVE"
    ///   12-15: "fmt "
    ///   16-19: 16 (fmt chunk size)
    ///   20-21: 1 (PCM format)
    ///   22-23: channel count
    ///   24-27: sample rate
    ///   28-31: byte rate (sampleRate * channels * bitsPerSample/8)
    ///   32-33: block align (channels * bitsPerSample/8)
    ///   34-35: bits per sample
    ///   36-39: "data"
    ///   40-43: data size
    private func buildWAVHeader(dataSize: UInt32) -> Data {
        let sampleRate = UInt32(OutputFormat.sampleRate)
        let channelCount: UInt16 = UInt16(OutputFormat.channels)
        let bitsPerSample: UInt16 = UInt16(OutputFormat.bitsPerSample)
        let byteRate = sampleRate * UInt32(channelCount) * UInt32(bitsPerSample) / 8
        let blockAlign = channelCount * bitsPerSample / 8

        var header = Data(capacity: 44)

        // RIFF chunk
        header.append(contentsOf: "RIFF".utf8)
        var riffSize = dataSize == 0xFFFFFFFF ? dataSize : dataSize + 36
        header.append(Data(bytes: &riffSize, count: 4))
        header.append(contentsOf: "WAVE".utf8)

        // fmt sub-chunk
        header.append(contentsOf: "fmt ".utf8)
        var fmtChunkSize: UInt32 = 16
        header.append(Data(bytes: &fmtChunkSize, count: 4))
        var audioFormat: UInt16 = 1  // PCM
        header.append(Data(bytes: &audioFormat, count: 2))
        var channelCountValue = channelCount
        header.append(Data(bytes: &channelCountValue, count: 2))
        var sampleRateValue = sampleRate
        header.append(Data(bytes: &sampleRateValue, count: 4))
        var byteRateValue = byteRate
        header.append(Data(bytes: &byteRateValue, count: 4))
        var blockAlignValue = blockAlign
        header.append(Data(bytes: &blockAlignValue, count: 2))
        var bitsPerSampleValue = bitsPerSample
        header.append(Data(bytes: &bitsPerSampleValue, count: 2))

        // data sub-chunk
        header.append(contentsOf: "data".utf8)
        var dataSizeValue = dataSize
        header.append(Data(bytes: &dataSizeValue, count: 4))

        return header
    }
}

// MARK: - Stdout Writer

/// Writes raw PCM data to stdout for piping to other tools.
/// No WAV header — just raw 16-bit signed integer samples.
final class StdoutWriter: AudioWriting, @unchecked Sendable {
    func prepare() throws {
        // Nothing to set up
    }

    func write(_ data: Data) throws {
        FileHandle.standardOutput.write(data)
    }

    func finalize() throws {
        // Nothing to close
    }
}