import AppKit
import Foundation
import UserNotifications

@MainActor
final class NotificationService: NSObject, UNUserNotificationCenterDelegate {
    static let shared = NotificationService()

    private static let categoryID = "SYNTRIX_INACTIVE_APP_MEMORY"
    private static let cleanActionID = "CLEAN_MEMORY"
    private static let ignoreActionID = "IGNORE"

    var onCleanMemory: ((String) -> Void)?
    private var isAuthorized = false
    private var isConfigured = false

    private override init() {
        super.init()
    }

    func configure() {
        guard !isConfigured else { return }
        isConfigured = true

        guard Bundle.main.bundleIdentifier != nil else {
            return
        }

        let center = UNUserNotificationCenter.current()
        center.delegate = self

        let cleanAction = UNNotificationAction(
            identifier: Self.cleanActionID,
            title: "Clean Memory",
            options: [.foreground]
        )
        let ignoreAction = UNNotificationAction(
            identifier: Self.ignoreActionID,
            title: "Ignore",
            options: []
        )
        let category = UNNotificationCategory(
            identifier: Self.categoryID,
            actions: [cleanAction, ignoreAction],
            intentIdentifiers: [],
            options: []
        )
        center.setNotificationCategories([category])

        center.requestAuthorization(options: [.alert, .sound, .badge]) { granted, _ in
            Task { @MainActor in
                self.isAuthorized = granted
            }
        }
    }

    func postInactiveAppNotification(_ info: InactiveAppNotificationInfo) {
        configure()

        if Bundle.main.bundleIdentifier != nil && isAuthorized {
            let content = UNMutableNotificationContent()
            content.title = info.title
            if let subtitle = info.subtitle, !subtitle.isEmpty {
                content.subtitle = subtitle
            }
            content.body = info.body
            content.sound = .default
            content.categoryIdentifier = Self.categoryID
            content.userInfo = [
                "rec_id": info.recId,
                "target_pid": Int(info.targetPid),
                "target_name": info.targetName
            ]

            let req = UNNotificationRequest(identifier: info.id, content: content, trigger: nil)
            UNUserNotificationCenter.current().add(req) { err in
                if err != nil {
                    Task { @MainActor in
                        self.postAppleScriptFallback(info)
                    }
                }
            }
            return
        }

        postAppleScriptFallback(info)
    }

    private func postAppleScriptFallback(_ info: InactiveAppNotificationInfo) {
        let escapedTitle = info.title.replacingOccurrences(of: "\"", with: "\\\"")
        let escapedSub = (info.subtitle ?? "").replacingOccurrences(of: "\"", with: "\\\"")
        let escapedBody = info.body.replacingOccurrences(of: "\"", with: "\\\"")
        let script = "display notification \"\(escapedBody)\" with title \"\(escapedTitle)\" subtitle \"\(escapedSub)\""

        let proc = Process()
        proc.executableURL = URL(fileURLWithPath: "/usr/bin/osascript")
        proc.arguments = ["-e", script]
        proc.standardOutput = FileHandle.nullDevice
        proc.standardError = FileHandle.nullDevice
        try? proc.run()
    }

    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        completionHandler([.banner, .sound, .list])
    }

    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse,
        withCompletionHandler completionHandler: @escaping () -> Void
    ) {
        let actionID = response.actionIdentifier
        let recID = response.notification.request.content.userInfo["rec_id"] as? String ?? ""

        Task { @MainActor in
            if actionID == Self.cleanActionID, !recID.isEmpty {
                self.onCleanMemory?(recID)
            } else if actionID == UNNotificationDefaultActionIdentifier {
                NSApp.activate(ignoringOtherApps: true)
            }
        }
        completionHandler()
    }
}
