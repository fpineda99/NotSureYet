import CoreAudio
import AVFAudio

/// Manages the lifecycle of a Core Audio process tap and aggregate device.
///
/// This class handles all the Apple API plumbing to capture system audio.
/// It knows nothing about what happens to the audio data — the IO block
/// is injected by the caller.
///
/// Lifecycle: createTap() → read tapFormat → startStreaming(ioBlock:) → stop()
final class AudioTapManager: @unchecked Sendable {
    private var tapID: AudioObjectID = .init(kAudioObjectUnknown)
    private var aggregateDeviceID: AudioObjectID = .init(kAudioObjectUnknown)
    private var ioProcID: AudioDeviceIOProcID?
    private var isRunning = false

    // MARK: - Public Interface

    /// The audio format coming from the tap (sample rate, channels, etc).
    /// Only valid after createTap() has been called.
    var tapFormat: AudioStreamBasicDescription {
        get throws {
            var format = AudioStreamBasicDescription()
            var size = UInt32(MemoryLayout<AudioStreamBasicDescription>.size)

            var address = AudioObjectPropertyAddress(
                mSelector: kAudioTapPropertyFormat,
                mScope: kAudioObjectPropertyScopeOutput,
                mElement: kAudioObjectPropertyElementMain
            )

            let status = AudioObjectGetPropertyData(
                tapID, &address, 0, nil, &size, &format
            )

            guard status == noErr else {
                throw AudioCaptureError.formatError(
                    "Could not read tap format (OSStatus \(status))"
                )
            }
            return format
        }
    }

    /// Step 1: Create the audio tap and aggregate device.
    /// After this call, you can read `tapFormat` to know the native sample rate.
    func createTap() throws {
        // Create a tap that captures ALL system audio, mixed to mono.
        // .unmuted means audio still plays through speakers (we're listening, not hijacking).
        let tapDescription = CATapDescription(monoGlobalTapButExcludeProcesses: [])
        tapDescription.muteBehavior = .unmuted
        tapDescription.name = "AudioCapture"

        var status = AudioHardwareCreateProcessTap(tapDescription, &tapID)
        guard status == noErr else {
            throw AudioCaptureError.tapCreationFailed(status)
        }

        // Read the tap's UUID — we need it to build the aggregate device
        let tapUID = try readTapUID()

        // Build the aggregate device that wraps our tap.
        // Think of this as a virtual microphone that reads from the tap.
        let aggregateDescription: NSDictionary = [
            kAudioAggregateDeviceNameKey: "AudioCapture",
            kAudioAggregateDeviceUIDKey: "AudioCapture-\(UUID().uuidString)",
            kAudioAggregateDeviceIsPrivateKey: 1,
            kAudioAggregateDeviceTapAutoStartKey: 1,
            kAudioAggregateDeviceTapListKey: [
                [kAudioSubTapUIDKey: tapUID]
            ]
        ]

        status = AudioHardwareCreateAggregateDevice(
            aggregateDescription as CFDictionary, &aggregateDeviceID
        )
        guard status == noErr else {
            throw AudioCaptureError.aggregateDeviceCreationFailed(status)
        }
    }

    /// Step 2: Start streaming audio through the provided IO block.
    /// The block is called on the real-time audio thread — do minimal work in it.
    func startStreaming(ioBlock: @escaping AudioDeviceIOBlock) throws {
        var status = AudioDeviceCreateIOProcIDWithBlock(
            &ioProcID, aggregateDeviceID, nil, ioBlock
        )
        guard status == noErr else {
            throw AudioCaptureError.ioProcCreationFailed(status)
        }

        status = AudioDeviceStart(aggregateDeviceID, ioProcID)
        guard status == noErr else {
            throw AudioCaptureError.deviceStartFailed(status)
        }

        isRunning = true
    }

    /// Tear everything down. Safe to call multiple times.
    func stop() {
        guard isRunning else { return }
        isRunning = false

        if let proc = ioProcID {
            AudioDeviceStop(aggregateDeviceID, proc)
            AudioDeviceDestroyIOProcID(aggregateDeviceID, proc)
            ioProcID = nil
        }

        if aggregateDeviceID != kAudioObjectUnknown {
            AudioHardwareDestroyAggregateDevice(aggregateDeviceID)
            aggregateDeviceID = .init(kAudioObjectUnknown)
        }

        if tapID != kAudioObjectUnknown {
            AudioHardwareDestroyProcessTap(tapID)
            tapID = .init(kAudioObjectUnknown)
        }
    }

    deinit {
        stop()
    }

    // MARK: - Private Helpers

    /// Read the UUID string from the tap, needed to reference it in the aggregate device.
    private func readTapUID() throws -> String {
        var address = AudioObjectPropertyAddress(
            mSelector: kAudioTapPropertyUID,
            mScope: kAudioObjectPropertyScopeGlobal,
            mElement: kAudioObjectPropertyElementMain
        )

        var uid: Unmanaged<CFString>?
        var size = UInt32(MemoryLayout<CFString?>.size)

        let status = AudioObjectGetPropertyData(
            tapID, &address, 0, nil, &size, &uid
        )

        guard status == noErr, let uidValue = uid?.takeUnretainedValue() else {
            throw AudioCaptureError.formatError(
                "Could not read tap UID (OSStatus \(status))"
            )
        }
        return uidValue as String
    }
}