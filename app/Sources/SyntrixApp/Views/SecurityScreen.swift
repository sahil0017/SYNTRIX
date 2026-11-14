import AppKit
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

    private var grantedCount: Int {
        var count = 1 // Keychain / local sandbox
        if screenCaptureState == .granted { count += 1 }
        if accessibilityState == .granted { count += 1 }
        if fullDiskAccessState == .granted { count += 1 }
        return count
    }

    private var filteredAuditLogs: [AuditLogItem] {
        let filter = client.securityAuditFilter
        let q = client.securityAuditSearch.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        return client.auditLogs.filter { log in
            let matchesActor: Bool
            switch filter {
            case "USER":
                matchesActor = log.actor.lowercased().contains("user")
            case "GUARDIAN":
                matchesActor = log.actor.lowercased().contains("guardian") || log.actor.lowercased().contains("executor")
            case "SYSTEM":
                matchesActor = log.actor.lowercased().contains("system") || log.actor.lowercased().contains("companion")
            default:
                matchesActor = true
            }
            if !matchesActor { return false }
            if client.securityAuditIssuesOnly {
                let s = log.status.lowercased()
                if s == "ok" || s == "verified" || s == "approved" {
                    return false
                }
            }
            if q.isEmpty { return true }
            return log.action.lowercased().contains(q)
                || log.target.lowercased().contains(q)
                || log.detail.lowercased().contains(q)
                || log.actor.lowercased().contains(q)
        }
    }

    var body: some View {
        VStack(spacing: 12) {
            securityPostureStrip

            HStack(alignment: .top, spacing: 12) {
                leftPermissionsColumn
                    .frame(width: 440)
                    .frame(maxHeight: .infinity, alignment: .top)

                rightAuditColumn
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
    }

    private var securityPostureStrip: some View {
        HStack(spacing: 10) {
            postureCard(
                title: "EXECUTION BOUNDARY",
                value: "LOCAL-FIRST",
                subtitle: "On-device SQLite (~/.syntrix/syntrix.db)",
                icon: "lock.shield.fill",
                color: HUDColors.green
            )
            postureCard(
                title: "POLICY GATE",
                value: client.mode.rawValue,
                subtitle: "Guardian: \(client.guardianStatus?.state ?? "ACTIVE")",
                icon: "checkmark.shield.fill",
                color: HUDColors.cyan
            )
            postureCard(
                title: "TCC PERMISSIONS",
                value: "\(grantedCount) / 4 GRANTED",
                subtitle: grantedCount == 4 ? "Full telemetry & context enabled" : "Grant optional permissions below",
                icon: "hand.raised.fill",
                color: grantedCount >= 3 ? HUDColors.green : HUDColors.amber
            )
            postureCard(
                title: "AUDIT TRAIL",
                value: "\(client.auditLogs.count) EVENTS",
                subtitle: client.auditLogs.first.map { "Last: \($0.action) (\($0.status))" } ?? "No actions logged",
                icon: "list.bullet.clipboard.fill",
                color: HUDColors.cyan
            )
        }
    }

    private func postureCard(title: String, value: String, subtitle: String, icon: String, color: Color) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Image(systemName: icon)
                    .font(.system(size: 10))
                    .foregroundStyle(color)
                Text(title)
                    .font(.system(size: 8.5, weight: .bold, design: .monospaced))
                    .foregroundStyle(HUDColors.textMuted)
                Spacer()
            }
            Text(value)
                .font(.system(size: 13, weight: .bold, design: .monospaced))
                .foregroundStyle(color)
            Text(subtitle)
                .font(.system(size: 9, design: .monospaced))
                .foregroundStyle(HUDColors.textMuted)
                .lineLimit(1)
        }
        .padding(9)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(HUDColors.panelBg)
        .overlay(RoundedRectangle(cornerRadius: 4).stroke(color.opacity(0.35), lineWidth: 1))
        .overlay(HUDCorners())
    }

    private var leftPermissionsColumn: some View {
        VStack(spacing: 12) {
            HUDPanel(title: "TCC PERMISSIONS & ACCESS", trailing: "\(grantedCount)/4") {
                VStack(spacing: 8) {
                    permissionCard(
                        icon: "rectangle.dashed.badge.record",
                        name: "Screen Capture",
                        purpose: "Window title & active context awareness",
                        state: screenCaptureState,
                        actionTitle: screenCaptureState == .granted ? "SETTINGS" : "REQUEST"
                    ) {
                        if screenCaptureState == .granted {
                            openPrivacyPane("Privacy_ScreenCapture")
                        } else {
                            UserDefaults.standard.set(true, forKey: "syntrix.screenCaptureRequested")
                            _ = CGRequestScreenCaptureAccess()
                        }
                    }

                    permissionCard(
                        icon: "accessibility",
                        name: "Accessibility",
                        purpose: "App responsiveness & focus window diagnostics",
                        state: accessibilityState,
                        actionTitle: accessibilityState == .granted ? "SETTINGS" : "REQUEST"
                    ) {
                        if accessibilityState == .granted {
                            openPrivacyPane("Privacy_Accessibility")
                        } else {
                            UserDefaults.standard.set(true, forKey: "syntrix.accessibilityRequested")
                            let opts = ["AXTrustedCheckOptionPrompt": true] as CFDictionary
                            _ = AXIsProcessTrustedWithOptions(opts)
                        }
                    }

                    permissionCard(
                        icon: "internaldrive",
                        name: "Full Disk Access",
                        purpose: "Deep cache & DerivedData storage audits",
                        state: fullDiskAccessState,
                        actionTitle: "OPEN SETTINGS"
                    ) {
                        openPrivacyPane("Privacy_AllFiles")
                    }

                    permissionCard(
                        icon: "key.fill",
                        name: "Local Keychain & DB",
                        purpose: "Local SQLite state (~/.syntrix/syntrix.db)",
                        state: .granted,
                        actionTitle: nil,
                        action: {}
                    )
                }
            }

            HUDPanel(title: "PRIVACY & SAFETY GUARDRAILS", trailing: "PROTECTED") {
                VStack(alignment: .leading, spacing: 8) {
                    Toggle("NSWorkspace Active App Context", isOn: $client.contextCaptureEnabled)
                        .font(.system(size: 10.5, design: .monospaced))
                        .foregroundStyle(HUDColors.cyan)
                        .toggleStyle(.switch)

                    Divider().overlay(HUDColors.cyanDim)

                    Text("PROTECTED SYSTEM PROCESSES (NEVER TERMINATED)")
                        .font(.system(size: 8.5, weight: .bold, design: .monospaced))
                        .foregroundStyle(HUDColors.textMuted)

                    HStack(spacing: 6) {
                        ForEach(["kernel_task", "launchd", "WindowServer", "Finder", "Dock", "syntrixd"], id: \.self) { proc in
                            Text(proc)
                                .font(.system(size: 8.5, design: .monospaced))
                                .foregroundStyle(HUDColors.green)
                                .padding(.horizontal, 5)
                                .padding(.vertical, 2)
                                .background(HUDColors.green.opacity(0.10))
                                .cornerRadius(3)
                        }
                    }

                    HStack {
                        Text("Capability-gated actions require explicit policy approval.")
                            .font(.system(size: 9, design: .monospaced))
                            .foregroundStyle(HUDColors.textMuted)
                        Spacer()
                        Button {
                            client.activateKillSwitch()
                        } label: {
                            Label("STOP ALL", systemImage: "stop.circle.fill")
                                .font(.system(size: 9, weight: .bold, design: .monospaced))
                                .foregroundStyle(HUDColors.red)
                                .padding(.horizontal, 8)
                                .padding(.vertical, 4)
                                .background(HUDColors.red.opacity(0.14))
                                .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.red.opacity(0.5), lineWidth: 1))
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
        }
    }

    private func permissionCard(
        icon: String,
        name: String,
        purpose: String,
        state: PermissionState,
        actionTitle: String?,
        action: @escaping () -> Void
    ) -> some View {
        HStack(spacing: 10) {
            Image(systemName: icon)
                .font(.system(size: 13))
                .foregroundStyle(state.color)
                .frame(width: 22)

            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Text(name)
                        .font(.system(size: 10.5, weight: .bold, design: .monospaced))
                        .foregroundStyle(HUDColors.textPrimary)
                    Text(state.rawValue)
                        .font(.system(size: 8, weight: .bold, design: .monospaced))
                        .foregroundStyle(state.color)
                        .padding(.horizontal, 5)
                        .padding(.vertical, 1)
                        .background(state.color.opacity(0.14))
                        .cornerRadius(2)
                }
                Text(purpose)
                    .font(.system(size: 9, design: .monospaced))
                    .foregroundStyle(HUDColors.textMuted)
                    .lineLimit(1)
            }

            Spacer()

            if let title = actionTitle {
                Button(title, action: action)
                    .font(.system(size: 8.5, weight: .bold, design: .monospaced))
                    .foregroundStyle(state == .granted ? HUDColors.cyan : HUDColors.amber)
                    .padding(.horizontal, 8)
                    .padding(.vertical, 4)
                    .overlay(RoundedRectangle(cornerRadius: 3).stroke((state == .granted ? HUDColors.cyan : HUDColors.amber).opacity(0.6), lineWidth: 1))
                    .buttonStyle(.plain)
            }
        }
        .padding(7)
        .background(HUDColors.background.opacity(0.45))
        .overlay(RoundedRectangle(cornerRadius: 4).stroke(HUDColors.cyanDim, lineWidth: 1))
    }

    private var rightAuditColumn: some View {
        let logs = filteredAuditLogs
        let okCount = client.auditLogs.filter {
            let s = $0.status.lowercased()
            return s == "ok" || s == "verified" || s == "approved"
        }.count
        let issueCount = client.auditLogs.count - okCount
        let guardCount = client.auditLogs.filter {
            let a = $0.actor.lowercased()
            return a.contains("guardian") || a.contains("executor")
        }.count

        return HUDPanel(title: "SQLITE AUDIT TRAIL", trailing: "\(logs.count) OF \(client.auditLogs.count) ENTRIES") {
            VStack(spacing: 8) {
                // Summary Strip + Export/Copy
                HStack(spacing: 8) {
                    auditStatBadge("VERIFIED/OK", "\(okCount)", HUDColors.green)
                    auditStatBadge("GUARDIAN", "\(guardCount)", HUDColors.cyan)
                    auditStatBadge("ISSUES", "\(issueCount)", issueCount > 0 ? HUDColors.red : HUDColors.textMuted)
                    Spacer()
                    Button {
                        copyAuditReport(logs)
                    } label: {
                        HStack(spacing: 4) {
                            Image(systemName: client.auditCopiedFeedback ? "checkmark" : "doc.on.doc")
                                .font(.system(size: 9))
                            Text(client.auditCopiedFeedback ? "COPIED" : "COPY REPORT")
                                .font(.system(size: 8.5, weight: .bold, design: .monospaced))
                        }
                        .foregroundStyle(client.auditCopiedFeedback ? HUDColors.green : HUDColors.cyan)
                        .padding(.horizontal, 8)
                        .padding(.vertical, 4)
                        .background(HUDColors.cyan.opacity(0.10))
                        .overlay(RoundedRectangle(cornerRadius: 3).stroke((client.auditCopiedFeedback ? HUDColors.green : HUDColors.cyan).opacity(0.5), lineWidth: 1))
                    }
                    .buttonStyle(.plain)
                }

                // Filter & Search Controls
                HStack(spacing: 5) {
                    ForEach(["ALL", "USER", "GUARDIAN", "SYSTEM"], id: \.self) { pill in
                        let active = client.securityAuditFilter == pill
                        let pillStyle = filterPillStyle(pill)
                        Button {
                            client.securityAuditFilter = pill
                        } label: {
                            HStack(spacing: 4) {
                                Image(systemName: pillStyle.icon)
                                    .font(.system(size: 8.5, weight: .semibold))
                                Text(pill)
                                    .font(.system(size: 8.5, weight: .bold, design: .monospaced))
                                    .lineLimit(1)
                            }
                            .foregroundStyle(active ? pillStyle.color : HUDColors.textMuted)
                            .padding(.horizontal, 7)
                            .padding(.vertical, 4)
                            .background(active ? pillStyle.color.opacity(0.16) : HUDColors.background.opacity(0.5))
                            .overlay(RoundedRectangle(cornerRadius: 3).stroke(active ? pillStyle.color : HUDColors.cyanDim, lineWidth: 1))
                            .fixedSize(horizontal: true, vertical: false)
                        }
                        .buttonStyle(.plain)
                    }

                    Button {
                        client.securityAuditIssuesOnly.toggle()
                    } label: {
                        HStack(spacing: 4) {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .font(.system(size: 8))
                            Text("ISSUES")
                                .font(.system(size: 8.5, weight: .bold, design: .monospaced))
                                .lineLimit(1)
                        }
                        .foregroundStyle(client.securityAuditIssuesOnly ? HUDColors.amber : HUDColors.textMuted)
                        .padding(.horizontal, 7)
                        .padding(.vertical, 4)
                        .background(client.securityAuditIssuesOnly ? HUDColors.amber.opacity(0.16) : HUDColors.background.opacity(0.5))
                        .overlay(RoundedRectangle(cornerRadius: 3).stroke(client.securityAuditIssuesOnly ? HUDColors.amber : HUDColors.cyanDim, lineWidth: 1))
                        .fixedSize(horizontal: true, vertical: false)
                    }
                    .buttonStyle(.plain)

                    Spacer(minLength: 4)

                    HStack(spacing: 5) {
                        Image(systemName: "magnifyingglass")
                            .font(.system(size: 9))
                            .foregroundStyle(HUDColors.textMuted)
                        TextField("Filter...", text: $client.securityAuditSearch)
                            .textFieldStyle(.plain)
                            .font(.system(size: 10, design: .monospaced))
                            .foregroundStyle(HUDColors.textPrimary)
                            .frame(minWidth: 80, maxWidth: 120)
                        if !client.securityAuditSearch.isEmpty {
                            Button {
                                client.securityAuditSearch = ""
                            } label: {
                                Image(systemName: "xmark.circle.fill")
                                    .font(.system(size: 9))
                                    .foregroundStyle(HUDColors.textMuted)
                            }
                            .buttonStyle(.plain)
                        }
                    }
                    .padding(.horizontal, 7)
                    .padding(.vertical, 4)
                    .background(HUDColors.background.opacity(0.6))
                    .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.cyanDim, lineWidth: 1))
                }

                if logs.isEmpty {
                    Spacer()
                    Text("No matching audit entries.")
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundStyle(HUDColors.textMuted)
                    Spacer()
                } else {
                    ScrollView {
                        VStack(alignment: .leading, spacing: 5) {
                            ForEach(logs) { log in
                                let s = log.status.lowercased()
                                let isOk = (s == "ok" || s == "verified" || s == "approved")
                                let isErr = (s == "error" || s == "denied" || s == "failed")
                                let statusColor = isOk ? HUDColors.green : (isErr ? HUDColors.red : HUDColors.amber)
                                let isExpanded = client.selectedAuditLogID == log.id
                                let actor = actorStyle(log.actor)

                                Button {
                                    client.selectedAuditLogID = isExpanded ? nil : log.id
                                } label: {
                                    HStack(alignment: .top, spacing: 8) {
                                        RoundedRectangle(cornerRadius: 1.5)
                                            .fill(statusColor)
                                            .frame(width: 3)
                                            .padding(.vertical, 2)

                                        VStack(alignment: .leading, spacing: 3) {
                                            HStack(spacing: 6) {
                                                Text(HUDFormatters.time.string(from: Date(timeIntervalSince1970: Double(log.timestampMs) / 1000.0)))
                                                    .font(.system(size: 9, design: .monospaced))
                                                    .foregroundStyle(HUDColors.textMuted)
                                                    .lineLimit(1)
                                                    .fixedSize(horizontal: true, vertical: false)

                                                HStack(spacing: 4) {
                                                    Image(systemName: actor.icon)
                                                        .font(.system(size: 8.5, weight: .semibold))
                                                    Text(log.actor.uppercased())
                                                        .font(.system(size: 8, weight: .bold, design: .monospaced))
                                                        .lineLimit(1)
                                                }
                                                .foregroundStyle(actor.color)
                                                .padding(.horizontal, 6)
                                                .padding(.vertical, 2)
                                                .background(actor.color.opacity(0.14))
                                                .overlay(RoundedRectangle(cornerRadius: 3).stroke(actor.color.opacity(0.35), lineWidth: 1))
                                                .cornerRadius(3)
                                                .fixedSize(horizontal: true, vertical: false)

                                                Text("\(log.action) → \(log.target)")
                                                    .font(.system(size: 10, weight: .semibold, design: .monospaced))
                                                    .foregroundStyle(HUDColors.textPrimary)
                                                    .lineLimit(1)
                                                    .truncationMode(.tail)

                                                Spacer(minLength: 4)

                                                Text(log.status.uppercased())
                                                    .font(.system(size: 8, weight: .bold, design: .monospaced))
                                                    .foregroundStyle(statusColor)
                                                    .lineLimit(1)
                                                    .padding(.horizontal, 5)
                                                    .padding(.vertical, 1)
                                                    .background(statusColor.opacity(0.12))
                                                    .cornerRadius(2)
                                                    .fixedSize(horizontal: true, vertical: false)

                                                Image(systemName: isExpanded ? "chevron.up" : "chevron.down")
                                                    .font(.system(size: 8))
                                                    .foregroundStyle(HUDColors.textMuted)
                                            }

                                            if !log.detail.isEmpty {
                                                Text(log.detail)
                                                    .font(.system(size: 9.5, design: .monospaced))
                                                    .foregroundStyle(isExpanded ? HUDColors.textPrimary : HUDColors.textMuted)
                                                    .lineLimit(isExpanded ? nil : 1)
                                                    .multilineTextAlignment(.leading)
                                            }
                                        }
                                    }
                                    .padding(.vertical, 5)
                                    .padding(.horizontal, 7)
                                    .background(isExpanded ? HUDColors.cyan.opacity(0.08) : HUDColors.background.opacity(0.35))
                                    .overlay(
                                        RoundedRectangle(cornerRadius: 4)
                                            .stroke(isExpanded ? HUDColors.cyan.opacity(0.45) : Color.clear, lineWidth: 1)
                                    )
                                    .cornerRadius(4)
                                    .contentShape(Rectangle())
                                }
                                .buttonStyle(.plain)
                            }
                        }
                    }
                    .frame(maxHeight: .infinity)
                }
            }
        }
    }

    private func auditStatBadge(_ label: String, _ value: String, _ color: Color) -> some View {
        HStack(spacing: 4) {
            Text(label)
                .font(.system(size: 8, weight: .bold, design: .monospaced))
                .foregroundStyle(HUDColors.textMuted)
            Text(value)
                .font(.system(size: 9.5, weight: .bold, design: .monospaced))
                .foregroundStyle(color)
        }
        .padding(.horizontal, 7)
        .padding(.vertical, 3)
        .background(color.opacity(0.08))
        .overlay(RoundedRectangle(cornerRadius: 3).stroke(color.opacity(0.3), lineWidth: 1))
    }

    private func copyAuditReport(_ logs: [AuditLogItem]) {
        let lines = logs.map { log in
            let ts = HUDFormatters.time.string(from: Date(timeIntervalSince1970: Double(log.timestampMs) / 1000.0))
            return "[\(ts)] [\(log.actor)] \(log.action) -> \(log.target) (\(log.status)): \(log.detail)"
        }
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(lines.joined(separator: "\n"), forType: .string)
        client.auditCopiedFeedback = true
        DispatchQueue.main.asyncAfter(deadline: .now() + 1.5) {
            client.auditCopiedFeedback = false
        }
    }

    private func filterPillStyle(_ pill: String) -> (icon: String, color: Color) {
        switch pill {
        case "USER":
            return ("person.fill", HUDColors.cyan)
        case "GUARDIAN":
            return ("shield.lefthalf.filled", HUDColors.green)
        case "SYSTEM":
            return ("gearshape.fill", HUDColors.amber)
        default:
            return ("tray.full.fill", HUDColors.cyan)
        }
    }

    private func actorStyle(_ actor: String) -> (icon: String, color: Color) {
        let lower = actor.lowercased()
        if lower.contains("user") {
            return ("person.fill", HUDColors.cyan)
        }
        if lower.contains("guardian") || lower.contains("executor") {
            return ("shield.lefthalf.filled", HUDColors.green)
        }
        return ("gearshape.fill", HUDColors.amber)
    }

    private func openPrivacyPane(_ anchor: String) {
        if let url = URL(string: "x-apple.systempreferences:com.apple.preference.security?\(anchor)") {
            NSWorkspace.shared.open(url)
        }
    }
}
