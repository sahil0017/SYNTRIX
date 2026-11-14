// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "SyntrixApp",
    platforms: [
        .macOS(.v14)
    ],
    products: [
        .executable(name: "SyntrixApp", targets: ["SyntrixApp"])
    ],
    targets: [
        .executableTarget(
            name: "SyntrixApp",
            path: "Sources/SyntrixApp"
        )
    ]
)
