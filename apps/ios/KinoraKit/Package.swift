// swift-tools-version: 6.0
// KinoraKit: the Kinora API client, models, server addresses, discovery and storage.
// Shared by the iPhone/iPad app, its widgets and the Apple TV app. No UIKit here, so `swift test` runs on a Mac.
import PackageDescription

let package = Package(
    name: "KinoraKit",
    defaultLocalization: "hu",
    platforms: [.iOS(.v17), .tvOS(.v17), .macOS(.v14)],
    products: [
        .library(name: "KinoraKit", targets: ["KinoraKit"])
    ],
    targets: [
        .target(name: "KinoraKit"),
        .testTarget(name: "KinoraKitTests", dependencies: ["KinoraKit"])
    ]
)
