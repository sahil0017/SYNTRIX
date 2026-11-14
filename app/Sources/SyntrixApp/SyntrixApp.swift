import AppKit
import SwiftUI

@main
struct SyntrixApp: App {
    private let client = SyntrixClient()
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate

    var body: some Scene {
        WindowGroup("SYNTRIX") {
            ContentView(client: client)
                .frame(minWidth: 1080, minHeight: 680)
                .onAppear {
                    NSApp.setActivationPolicy(.regular)
                    NSApp.appearance = NSAppearance(named: .darkAqua)
                    NSApp.activate(ignoringOtherApps: true)
                    appDelegate.ensureDaemonRunning()
                    NotificationService.shared.onCleanMemory = { recID in
                        client.approveRecommendation(recID)
                    }
                    NotificationService.shared.configure()
                    client.start()
                }
                .onDisappear {
                    client.stop()
                }
        }
        .windowStyle(.hiddenTitleBar)
    }
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    private var daemonProcess: Process?

    func ensureDaemonRunning() {
        let sockPath = "/tmp/syntrixd.sock"
        if FileManager.default.fileExists(atPath: sockPath) && canConnect(sockPath: sockPath) {
            return
        }

        guard let daemonURL = findDaemonBinary() else {
            return
        }

        let proc = Process()
        proc.executableURL = daemonURL
        proc.standardOutput = FileHandle.nullDevice
        proc.standardError = FileHandle.nullDevice
        do {
            try proc.run()
            daemonProcess = proc
        } catch {
            // Client will continue retrying connection if daemon is started externally.
        }
    }

    func applicationWillTerminate(_ notification: Notification) {
        clientTerminateDaemon()
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }

    private func clientTerminateDaemon() {
        if let proc = daemonProcess, proc.isRunning {
            proc.terminate()
        }
        daemonProcess = nil
    }

    private func findDaemonBinary() -> URL? {
        if let execURL = Bundle.main.executableURL {
            let sibling = execURL.deletingLastPathComponent().appendingPathComponent("syntrixd")
            if FileManager.default.isExecutableFile(atPath: sibling.path) {
                return sibling
            }
        }
        let candidates = [
            FileManager.default.currentDirectoryPath + "/runtime/build/syntrixd",
            "/Users/sahilkanungo/Desktop/SYNTRIX/runtime/build/syntrixd"
        ]
        for path in candidates where FileManager.default.isExecutableFile(atPath: path) {
            return URL(fileURLWithPath: path)
        }
        return nil
    }

    private func canConnect(sockPath: String) -> Bool {
        let sock = Darwin.socket(AF_UNIX, SOCK_STREAM, 0)
        guard sock >= 0 else { return false }
        defer { Darwin.close(sock) }

        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        let pathBytes = Array(sockPath.utf8CString)
        let maxLen = MemoryLayout.size(ofValue: addr.sun_path)
        guard pathBytes.count <= maxLen else { return false }
        withUnsafeMutablePointer(to: &addr.sun_path) { ptr in
            ptr.withMemoryRebound(to: CChar.self, capacity: maxLen) { dst in
                for i in 0..<pathBytes.count {
                    dst[i] = pathBytes[i]
                }
            }
        }
        let len = socklen_t(MemoryLayout<sockaddr_un>.size)
        let rc = withUnsafePointer(to: &addr) { ptr in
            ptr.withMemoryRebound(to: sockaddr.self, capacity: 1) { sa in
                Darwin.connect(sock, sa, len)
            }
        }
        return rc == 0
    }
}
