import ArgumentParser
import Dispatch
import Foundation

@main
struct AudioCapture: ParsableCommand {
    static let configuration = CommandConfiguration(
        abstract: "Capture macOS system audio to a WAV file.",
        discussion: "Records all system audio output using Core Audio Taps API. "
            + "Output is 16kHz mono 16-bit PCM, optimized for speech transcription.\n\n"
            + "Default recordings are saved to ~/.audiocapture/recordings/ with timestamp-based names."
    )

    @Option(name: .shortAndLong, help: "Output file path. Overrides default directory and naming.")
    var output: String?

    @Option(name: .shortAndLong, help: "Label appended to filename (e.g. 'lecture', 'meeting').")
    var label: String?

    @Option(name: .shortAndLong, help: "Recording duration in seconds (0 = until Ctrl+C).")
    var duration: Double = 0

    @Flag(name: .long, help: "Write raw PCM to stdout instead of a WAV file.")
    var rawStdout: Bool = false

    @Flag(name: .shortAndLong, help: "Verbose logging.")
    var verbose: Bool = false

    mutating func run() throws {
        let isStdout = rawStdout
        let recordDuration = duration
        let isVerbose = verbose
        let outputPath = try resolveOutputPath()

        func log(_ message: String) {
            if isVerbose {
                fputs("[AudioCapture] \(message)\n", stderr)
            }
        }

        // 1. Choose the writer based on output mode
        let writer: AudioWriting = isStdout
            ? StdoutWriter()
            : WAVFileWriter(path: outputPath)

        // 2. Create the tap and read its native format
        let tapManager = AudioTapManager()

        log("Creating audio tap...")
        try tapManager.createTap()

        let format = try tapManager.tapFormat
        log("Tap format: \(Int(format.mSampleRate))Hz, \(format.mChannelsPerFrame) channel(s) → \(Int(OutputFormat.sampleRate))Hz, \(OutputFormat.channels) channel(s) Int16")

        // 3. Set up the processor (handles format conversion)
        let processor = try AudioProcessor(tapFormat: format, writer: writer)

        // 4. Prepare the writer (create file, write WAV header)
        try writer.prepare()

        // 5. Start streaming audio
        log("Starting capture...")
        try tapManager.startStreaming(ioBlock: processor.makeIOBlock())

        let outputTarget = isStdout ? "stdout" : outputPath
        fputs("Recording to \(outputTarget). Press Ctrl+C to stop.\n", stderr)

        // 6. Set up clean shutdown.
        nonisolated(unsafe) var hasShutDown = false

        let shutdownWork = { @Sendable () -> Void in
            guard !hasShutDown else { return }
            hasShutDown = true

            fputs("\nStopping capture...\n", stderr)
            tapManager.stop()

            do {
                try writer.finalize()
            } catch {
                fputs("Warning: could not finalize output: \(error.localizedDescription)\n", stderr)
            }

            if !isStdout {
                if let attrs = try? FileManager.default.attributesOfItem(atPath: outputPath),
                   let size = attrs[.size] as? UInt64
                {
                    let kb = Double(size) / 1024.0
                    let seconds = Double(size - 44) / (OutputFormat.sampleRate * Double(OutputFormat.bytesPerSample))
                    fputs(String(format: "Saved %.1f KB (%.1f seconds) to %@\n", kb, seconds, outputPath), stderr)
                }
            }

            Darwin.exit(0)
        }

        // Handle Ctrl+C
        signal(SIGINT, SIG_IGN)
        let sigintSource = DispatchSource.makeSignalSource(signal: SIGINT, queue: .main)
        sigintSource.setEventHandler(handler: shutdownWork)
        sigintSource.resume()

        // Handle kill/termination
        signal(SIGTERM, SIG_IGN)
        let sigtermSource = DispatchSource.makeSignalSource(signal: SIGTERM, queue: .main)
        sigtermSource.setEventHandler(handler: shutdownWork)
        sigtermSource.resume()

        // 7. If a duration was specified, schedule auto-stop
        if recordDuration > 0 {
            fputs("Will stop after \(Int(recordDuration)) second(s).\n", stderr)
            DispatchQueue.main.asyncAfter(deadline: .now() + recordDuration, execute: shutdownWork)
        }

        // 8. Keep the process alive. dispatchMain() never returns —
        //    the process exits via Darwin.exit(0) in the shutdown handler.
        dispatchMain()
    }

    // MARK: - Path Resolution

    /// Determines the output file path.
    /// If --output is provided, use it as-is.
    /// Otherwise, create a session folder in ~/AudioCapture/ and save recording.wav inside it.
    ///
    /// Session folder structure:
    ///   ~/AudioCapture/2026-04-12_13-45-30_lecture/
    ///     recording.wav      ← this file
    ///     transcript.json    ← added by transcriber later
    ///     transcript.txt     ← added by transcriber later
    private func resolveOutputPath() throws -> String {
        // Custom path takes priority — no session folder
        if let customPath = output {
            return (customPath as NSString).expandingTildeInPath
        }

        // Build session folder name from timestamp + optional label
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        let baseDir = "\(home)/AudioCapture"

        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.dateFormat = "yyyy-MM-dd_HH-mm-ss"
        let timestamp = formatter.string(from: Date())

        let folderName: String
        if let label = label, !label.isEmpty {
            let safe = label.replacingOccurrences(of: "[^a-zA-Z0-9_-]", with: "-", options: .regularExpression)
            folderName = "\(timestamp)_\(safe)"
        } else {
            folderName = timestamp
        }

        let sessionDir = "\(baseDir)/\(folderName)"

        // Create session folder
        try FileManager.default.createDirectory(
            atPath: sessionDir,
            withIntermediateDirectories: true
        )

        return "\(sessionDir)/recording.wav"
    }
}
