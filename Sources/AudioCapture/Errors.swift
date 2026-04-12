import Foundation

/// All errors that can occur during audio capture.
/// Each case maps to a specific failure point in the pipeline,
/// with a human-readable description so you know what went wrong.
enum AudioCaptureError: LocalizedError {
    case tapCreationFailed(OSStatus)
    case aggregateDeviceCreationFailed(OSStatus)
    case ioProcCreationFailed(OSStatus)
    case deviceStartFailed(OSStatus)
    case formatError(String)
    case fileCreationFailed(String)

    var errorDescription: String? {
        switch self {
        case .tapCreationFailed(let status):
            return "Failed to create audio tap (OSStatus \(status)). "
                + "Check that Screen Recording permission is granted in "
                + "System Settings > Privacy & Security > Screen Recording."
        case .aggregateDeviceCreationFailed(let status):
            return "Failed to create aggregate audio device (OSStatus \(status))."
        case .ioProcCreationFailed(let status):
            return "Failed to create audio IO procedure (OSStatus \(status))."
        case .deviceStartFailed(let status):
            return "Failed to start audio device (OSStatus \(status))."
        case .formatError(let detail):
            return "Audio format error: \(detail)"
        case .fileCreationFailed(let path):
            return "Failed to create output file at: \(path)"
        }
    }
}
