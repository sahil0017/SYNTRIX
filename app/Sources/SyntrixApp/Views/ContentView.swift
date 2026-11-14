import SwiftUI

struct ContentView: View {
    @ObservedObject var client: SyntrixClient

    private var currentScreen: AppScreen {
        AppScreen(rawValue: client.activeTab) ?? .overview
    }

    var body: some View {
        ZStack {
            HUDColors.background.ignoresSafeArea()
            gridOverlay

            HStack(spacing: 0) {
                sidebar
                    .frame(width: 200)

                Divider()
                    .overlay(HUDColors.cyanDim)

                VStack(spacing: 14) {
                    topBar

                    Group {
                        switch currentScreen {
                        case .overview:
                            OverviewScreen(client: client)
                        case .assistant:
                            AssistantScreen(client: client)
                        case .telemetry:
                            TelemetryScreen(client: client)
                        case .research:
                            ResearchScreen(client: client)
                        case .workflows:
                            WorkflowsScreen(client: client)
                        case .security:
                            SecurityScreen(client: client)
                        }
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                }
                .padding(16)
            }
        }
        .frame(minWidth: 1080, minHeight: 680)
    }

    private var sidebar: some View {
        VStack(alignment: .leading, spacing: 14) {
            VStack(alignment: .leading, spacing: 4) {
                Text("SYNTRIX")
                    .font(.system(size: 18, weight: .bold, design: .monospaced))
                    .tracking(4)
                    .foregroundStyle(HUDColors.cyan)
                Text("MACOS INTELLIGENCE")
                    .font(.system(size: 9, weight: .medium, design: .monospaced))
                    .tracking(1.6)
                    .foregroundStyle(HUDColors.textMuted)
            }
            .padding(.top, 4)

            HStack(spacing: 6) {
                Circle()
                    .fill(client.isConnected ? HUDColors.green : HUDColors.amber)
                    .frame(width: 8, height: 8)
                Text(client.isConnected ? "DAEMON LIVE" : "CONNECTING...")
                    .font(.system(size: 9, weight: .bold, design: .monospaced))
                    .foregroundStyle(client.isConnected ? HUDColors.green : HUDColors.amber)
            }
            .padding(.horizontal, 8)
            .padding(.vertical, 4)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background((client.isConnected ? HUDColors.green : HUDColors.amber).opacity(0.12))
            .cornerRadius(4)

            Divider().overlay(HUDColors.cyanDim)

            VStack(spacing: 5) {
                ForEach(AppScreen.allCases) { screen in
                    let isSelected = currentScreen == screen
                    Button {
                        client.activeTab = screen.rawValue
                    } label: {
                        HStack(spacing: 8) {
                            Image(systemName: screen.icon)
                                .font(.system(size: 11))
                                .imageScale(.small)
                                .frame(width: 15)
                            Text(screen.rawValue.uppercased())
                                .font(.system(size: 11.5, weight: isSelected ? .bold : .medium, design: .monospaced))
                            Spacer()
                            if let badge = badgeCount(for: screen), badge > 0 {
                                Text("\(badge)")
                                    .font(.system(size: 9.5, weight: .bold, design: .monospaced))
                                    .padding(.horizontal, 5)
                                    .padding(.vertical, 2)
                                    .background(HUDColors.amber.opacity(0.2))
                                    .foregroundStyle(HUDColors.amber)
                                    .cornerRadius(3)
                            }
                        }
                        .padding(.horizontal, 9)
                        .padding(.vertical, 7)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(isSelected ? HUDColors.cyan.opacity(0.20) : Color.clear)
                        .foregroundStyle(isSelected ? HUDColors.cyan : HUDColors.textPrimary)
                        .overlay(
                            RoundedRectangle(cornerRadius: 4)
                                .stroke(isSelected ? HUDColors.cyan : Color.clear, lineWidth: 1)
                        )
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                }
            }

            Spacer()

            if !client.activeApp.isEmpty {
                HStack(spacing: 6) {
                    Image(systemName: "app.badge")
                        .font(.system(size: 10))
                        .foregroundStyle(HUDColors.amber)
                    Text("CTX: \(client.activeApp)")
                        .font(.system(size: 10, design: .monospaced))
                        .foregroundStyle(HUDColors.amber)
                        .lineLimit(1)
                }
            }

            if let gov = client.governorStatus {
                let govColor: Color = gov.state == "NORMAL" ? HUDColors.green : (gov.state == "CRITICAL" ? HUDColors.red : HUDColors.amber)
                VStack(alignment: .leading, spacing: 3) {
                    Text("RESOURCE GOVERNOR")
                        .font(.system(size: 8, weight: .bold, design: .monospaced))
                        .foregroundStyle(HUDColors.textMuted)
                    HStack {
                        Text(gov.state)
                            .font(.system(size: 11, weight: .bold, design: .monospaced))
                            .foregroundStyle(govColor)
                        Spacer()
                        Text(String(format: "%.1f MB", gov.goHeapAllocMB))
                            .font(.system(size: 9, design: .monospaced))
                            .foregroundStyle(HUDColors.textMuted)
                    }
                }
                .padding(8)
                .background(govColor.opacity(0.10))
                .overlay(RoundedRectangle(cornerRadius: 4).stroke(govColor.opacity(0.4), lineWidth: 1))
            }
        }
        .padding(14)
        .background(HUDColors.panelBg.opacity(0.6))
    }

    private var topBar: some View {
        HStack {
            VStack(alignment: .leading, spacing: 2) {
                Text(currentScreen.rawValue.uppercased())
                    .font(.system(size: 16, weight: .bold, design: .monospaced))
                    .tracking(2)
                    .foregroundStyle(HUDColors.cyan)
                Text(currentScreen.subtitle)
                    .font(.system(size: 11.5, design: .default))
                    .foregroundStyle(HUDColors.textMuted)
            }

            Spacer()

            VStack(alignment: .trailing, spacing: 5) {
                HStack(spacing: 6) {
                    Button {
                        client.setAutoMode(!client.autoMode)
                    } label: {
                        HStack(spacing: 4) {
                            Image(systemName: "wand.and.stars")
                                .resizable()
                                .scaledToFit()
                                .frame(width: 9, height: 9)
                            Text("AUTO")
                                .font(.system(size: 10, weight: client.autoMode ? .bold : .semibold, design: .monospaced))
                        }
                        .padding(.horizontal, 8)
                        .padding(.vertical, 5)
                        .background(client.autoMode ? Color.purple.opacity(0.24) : HUDColors.panelBg.opacity(0.7))
                        .foregroundStyle(client.autoMode ? Color.purple : HUDColors.textMuted)
                        .clipShape(RoundedRectangle(cornerRadius: 4))
                        .overlay(
                            RoundedRectangle(cornerRadius: 4)
                                .stroke(client.autoMode ? Color.purple : HUDColors.cyanDim, lineWidth: 1)
                        )
                    }
                    .buttonStyle(.plain)
                    .fixedSize(horizontal: true, vertical: false)
                    .help("Dynamic AUTO: automatically switches between ASSIST, AUTOPILOT, and LEARNING from live telemetry & active app")

                    HStack(spacing: 2) {
                        ForEach(OperatingMode.allCases) { m in
                            let isSelected = client.mode == m
                            let modeColor = colorForMode(m)
                            Button {
                                client.setMode(m)
                            } label: {
                                HStack(spacing: 4) {
                                    if isSelected && client.isConnected && m != .off {
                                        Circle()
                                            .fill(modeColor)
                                            .frame(width: 4, height: 4)
                                    }
                                    Image(systemName: iconForMode(m))
                                        .resizable()
                                        .scaledToFit()
                                        .frame(width: 9, height: 9)
                                    Text(m.rawValue)
                                        .font(.system(size: 10, weight: isSelected ? .bold : .medium, design: .monospaced))
                                }
                                .padding(.horizontal, 7)
                                .padding(.vertical, 4)
                                .background(isSelected ? modeColor.opacity(0.22) : Color.clear)
                                .foregroundStyle(isSelected ? modeColor : HUDColors.textMuted)
                                .clipShape(RoundedRectangle(cornerRadius: 3))
                                .overlay(
                                    RoundedRectangle(cornerRadius: 3)
                                        .stroke(isSelected ? modeColor.opacity(0.8) : Color.clear, lineWidth: 1)
                                )
                                .contentShape(Rectangle())
                            }
                            .buttonStyle(.plain)
                            .fixedSize(horizontal: true, vertical: false)
                            .help(descriptionForMode(m))
                        }
                    }
                    .padding(2)
                    .background(HUDColors.panelBg.opacity(0.75))
                    .clipShape(RoundedRectangle(cornerRadius: 5))
                    .overlay(
                        RoundedRectangle(cornerRadius: 5)
                            .stroke(HUDColors.cyanDim, lineWidth: 1)
                    )
                }
                HStack(spacing: 8) {
                    if client.autoMode {
                        Text("AUTO-ADAPTIVE")
                            .font(.system(size: 8.5, weight: .bold, design: .monospaced))
                            .padding(.horizontal, 5)
                            .padding(.vertical, 1)
                            .background(Color.purple.opacity(0.22))
                            .foregroundStyle(Color.purple)
                            .cornerRadius(2)
                    }
                    Text(liveModeSummary(client.mode))
                        .font(.system(size: 10.5, design: .monospaced))
                        .foregroundStyle(colorForMode(client.mode).opacity(0.95))
                }
            }
        }
    }

    private func iconForMode(_ mode: OperatingMode) -> String {
        switch mode {
        case .monitor: return "eye"
        case .assist: return "shield"
        case .autopilot: return "bolt.fill"
        case .learning: return "sparkles"
        case .off: return "power"
        }
    }

    private func colorForMode(_ mode: OperatingMode) -> Color {
        switch mode {
        case .monitor: return HUDColors.cyan
        case .assist: return HUDColors.amber
        case .autopilot: return HUDColors.green
        case .learning: return Color.purple
        case .off: return HUDColors.red
        }
    }

    private func descriptionForMode(_ mode: OperatingMode) -> String {
        switch mode {
        case .monitor: return "MONITOR: Read-only telemetry & forecasting (no actions)"
        case .assist: return "ASSIST: Proposes optimizations & waits for your approval"
        case .autopilot: return "AUTOPILOT: Autonomously executes safe optimizations"
        case .learning: return "LEARNING: Prioritizes research indexing & knowledge graph"
        case .off: return "OFF: Telemetry & background intelligence paused"
        }
    }

    private func liveModeSummary(_ mode: OperatingMode) -> String {
        let guardState = client.guardianStatus?.state ?? "IDLE"
        let pendingCount = client.recommendations.filter { $0.status == "pending" }.count
        switch mode {
        case .off:
            return "OFF: Telemetry & background intelligence paused"
        case .monitor:
            let procs = client.snapshot?.processCountTotal ?? 0
            return "MONITOR: Read-only telemetry • \(procs) procs tracked"
        case .assist:
            return "ASSIST: \(pendingCount) pending • Guard: \(guardState)"
        case .autopilot:
            let acts = client.guardianStatus?.actionsTaken ?? 0
            return "AUTOPILOT: \(acts) auto-actions • Guard: \(guardState) (12/hr cap)"
        case .learning:
            let acc = client.pythonForecast?.transformerRL?.accuracyPct ?? 0
            return String(format: "LEARNING: %d docs indexed • %.1f%% ML acc • 2x SQLite", client.knowledgeDocs, acc)
        }
    }

    private func badgeCount(for screen: AppScreen) -> Int? {
        switch screen {
        case .overview:
            return client.recommendations.filter { $0.status == "pending" }.count
        case .workflows:
            return client.workflowRuns.filter { $0.status == "awaiting_approval" }.count
        default:
            return nil
        }
    }

    private var gridOverlay: some View {
        Canvas { ctx, size in
            let step: CGFloat = 40
            var path = Path()
            var x: CGFloat = 0
            while x <= size.width {
                path.move(to: CGPoint(x: x, y: 0))
                path.addLine(to: CGPoint(x: x, y: size.height))
                x += step
            }
            var y: CGFloat = 0
            while y <= size.height {
                path.move(to: CGPoint(x: 0, y: y))
                path.addLine(to: CGPoint(x: size.width, y: y))
                y += step
            }
            ctx.stroke(path, with: .color(HUDColors.cyan.opacity(0.045)), lineWidth: 1)
        }
        .ignoresSafeArea()
        .allowsHitTesting(false)
    }
}
