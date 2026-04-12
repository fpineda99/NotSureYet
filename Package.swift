// swift-tools-version: 6.2

import PackageDescription

let package = Package(
    name: "AudioCapture",
    platforms: [
        .macOS(.v15)  // Core Audio Taps API requires macOS 14.4+, we target 15 for latest APIs
    ],
    dependencies: [
        .package(url: "https://github.com/apple/swift-argument-parser.git", from: "1.5.0"),
    ],
    targets: [
        .executableTarget(
            name: "AudioCapture",
            dependencies: [
                .product(name: "ArgumentParser", package: "swift-argument-parser"),
            ],
            linkerSettings: [
                .linkedFramework("CoreAudio"),
                .linkedFramework("AVFAudio"),
            ]
        ),
    ]
)
