import ApplicationServices
import CoreGraphics
import Foundation
import SwiftUI

private enum PermissionState: String {
    case granted = "GRANTED"
    case denied = "DENIED"
    case notDetermined = "NOT DETERMINED"
    case unavailable = "UNAVAILABLE"

    var color: Color {
        switch self {
        case .granted: return HUDColors.green
        case .denied: return HUDColors.red
        case .notDetermined: return HUDColors.amber
        case .unavailable: return HUDColors.textMuted
        }
    }
}

struct SecurityScreen: View {
    @ObservedObject var client: SyntrixClient

    private var screenCaptureState: PermissionState {
        if CGPreflightScreenCaptureAccess() { return .granted }
        return UserDefaults.standard.bool(forKey: "syntrix.screenCaptureRequested") ? .denied : .notDetermined
    }

    private var accessibilityState: PermissionState {
        if AXIsProcessTrusted() { return .granted }
        return UserDefaults.standard.bool(forKey: "syntrix.accessibilityRequested") ? .denied : .notDetermined
    }

    private var fullDiskAccessState: PermissionState {
        let tccPath = "/Library/Application Support/com.apple.TCC/TCC.db"
        guard FileManager.default.fileExists(atPath: tccPath) else { return .unavailable }
        return FileManager.default.isReadableFile(atPath: tccPath) ? .granted : .denied
    }

    var body: some View {
        VStack(spacing: 14) {
            HUDPanel(title: "PRIVACY, TCC PERMISSIONS & CONTEXT CAPTURE", trailing: "LOCAL-FIRST") {
                VStack(alignment: .leading, spacing: 12) {
                    Text("• Local-first execution: all telemetry, vector indexes & diagnostics stay on this Mac (~/.syntrix/syntrix.db).\n• Capability-gated tools: destructive or mutating actions require explicit policy approval.")
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundStyle(HUDColors.textMuted)

                    HStack(spacing: 10) {
                        permissionBadge(name: "SCREEN CAPTURE", state: screenCaptureState)
                        permissionBadge(name: "ACCESSIBILITY", state: accessibilityState)
                        permissionBadge(name: "FULL DISK", state: fullDiskAccessState)
                        permissionBadge(name: "KEYCHAIN", state: .granted)
                    }

                    HStack(spacing: 12) {
                        Toggle("NSWorkspace Active App Context", isOn: $client.contextCaptureEnabled)
                            .font(.system(size: 11, design: .monospaced))
                            .foregroundStyle(HUDColors.cyan)
                            .toggleStyle(.switch)

                        Spacer()

                        Button("REQUEST SCREEN") {
                            UserDefaults.standard.set(true, forKey: "syntrix.screenCaptureRequested")
                            _ = CGRequestScreenCaptureAccess()
                        }
                        .font(.system(size: 10, weight: .semibold, design: .monospaced))
                        .foregroundStyle(HUDColors.amber)
                        .padding(.horizontal, 10)
                        .padding(.vertical, 5)
                        .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.amber, lineWidth: 1))
                        .buttonStyle(.plain)

                        Button("REQUEST ACCESSIBILITY") {
                            UserDefaults.standard.set(true, forKey: "syntrix.accessibilityRequested")
                            let opts = ["AXTrustedCheckOptionPrompt": true] as CFDictionary
                            _ = AXIsProcessTrustedWithOptions(opts)
                        }
                        .font(.system(size: 10, weight: .semibold, design: .monospaced))
                        .foregroundStyle(HUDColors.amber)
                        .padding(.horizontal, 10)
                        .padding(.vertical, 5)
                        .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.amber, lineWidth: 1))
                        .buttonStyle(.plain)
                    }
                }
            }

            HUDPanel(title: "SQLITE AUDIT TRAIL", trailing: "\(client.auditLogs.count) ENTRIES") {
                if client.auditLogs.isEmpty {
                    Text("No audit entries recorded yet.")
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundStyle(HUDColors.textMuted)
                } else {
                    ScrollView {
                        VStack(alignment: .leading, spacing: 6) {
                            ForEach(client.auditLogs) { log in
                                HStack(alignment: .top, spacing: 8) {
                                    Text(HUDFormatters.time.string(from: Date(timeIntervalSince1970: Double(log.timestampMs) / 1000.0)))
                                        .foregroundStyle(HUDColors.textMuted)
                                    Text("[\(log.actor)]")
                                        .foregroundStyle(HUDColors.amber)
                                    Text("\(log.action) → \(log.target) (\(log.status)): \(log.detail)")
                                        .foregroundStyle(HUDColors.textPrimary)
                                    Spacer()
                                }
                                .font(.system(size: 10, design: .monospaced))
                                .padding(.vertical, 2)
                            }
                        }
                    }
                    .frame(maxHeight: .infinity)
                }
            }
        }
    }

    private func permissionBadge(name: String, state: PermissionState) -> some View {
        HStack(spacing: 5) {
            Circle()
                .fill(state.color)
                .frame(width: 7, height: 7)
            Text("\(name): \(state.rawValue)")
                .font(.system(size: 9, weight: .semibold, design: .monospaced))
                .foregroundStyle(state.color)
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 4)
        .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.cyanDim, lineWidth: 1))
    }
}
