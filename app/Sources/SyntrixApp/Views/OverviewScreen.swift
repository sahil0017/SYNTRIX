import SwiftUI

struct OverviewScreen: View {
    @ObservedObject var client: SyntrixClient

    var body: some View {
        VStack(spacing: 14) {
            HStack(alignment: .top, spacing: 14) {
                // Left column: Core Telemetry + Predictive Diagnosis
                VStack(spacing: 14) {
                    HUDPanel(title: "SYSTEM HEALTH", trailing: "CLICK BAR FOR INSIGHTS") {
                        let cpu = client.snapshot?.cpu.totalUsagePct ?? 0
                        let mem = client.snapshot?.memory.pressurePct ?? 0
                        let disk = client.snapshot?.disk.usedPct ?? 0
                        let bat = client.snapshot?.power.batteryPct ?? 100

                        VStack(spacing: 8) {
                            interactiveHealthBar(key: "CPU", valuePct: cpu, accent: cpu > 75 ? HUDColors.red : HUDColors.amber)
                            interactiveHealthBar(key: "MEM", valuePct: mem, accent: mem > 75 ? HUDColors.red : HUDColors.cyan)
                            interactiveHealthBar(key: "DSK", valuePct: disk, accent: disk > 75 ? HUDColors.red : (disk > 50 ? HUDColors.amber : HUDColors.cyan))
                            interactiveHealthBar(key: "BAT", valuePct: bat, accent: HUDColors.green)

                            healthInsightCard
                        }
                    }

                    HUDPanel(title: "PREDICTIVE RESOURCE INTELLIGENCE", trailing: client.pythonForecast.map { "\($0.sampleCount) SAMPLES · SKLEARN" } ?? "PYTHON ML") {
                        if let py = client.pythonForecast {
                            VStack(alignment: .leading, spacing: 8) {
                                // 1. Memory Pressure Forecast
                                forecastRow(
                                    icon: "memorychip",
                                    title: "Memory Pressure",
                                    detail: String(format: "Now %.0f%% → 5m %.0f%% → 15m %.0f%% (%@)",
                                                   py.memoryForecast.currentPct,
                                                   py.memoryForecast.predicted5mPct,
                                                   py.memoryForecast.predicted15mPct,
                                                   py.memoryForecast.minutesToCritical > 0 ? String(format: "%.0fm to 85%%", py.memoryForecast.minutesToCritical) : "stable"),
                                    risk: py.memoryForecast.risk,
                                    actionLabel: py.memoryForecast.risk == "low" ? nil : "Free RAM",
                                    actionPrompt: "/optimize-memory"
                                )

                                // 2. Storage Ceiling (50% Limit) & Forecast
                                let audit = client.storageAudit
                                let over50 = audit?.exceedsLimit ?? (py.storageForecast.currentPct > 50.0)
                                let reclaimText = (audit?.reclaimableMb ?? 0) > 0
                                    ? String(format: " · %.0f MB unused", audit?.reclaimableMb ?? 0)
                                    : ""
                                forecastRow(
                                    icon: "internaldrive",
                                    title: over50 ? "Storage > 50% Limit" : "Storage Guard (50% Max)",
                                    detail: py.storageForecast.daysUntil90Pct >= 0
                                        ? String(format: "%.1f%% full%@ · +%.2f%%/day · ~%dd to 90%%", py.storageForecast.currentPct, reclaimText, py.storageForecast.growthPctPerDay, py.storageForecast.daysUntil90Pct)
                                        : String(format: "%.1f%% full%@ · %+.2f%%/day", py.storageForecast.currentPct, reclaimText, py.storageForecast.growthPctPerDay),
                                    risk: over50 ? "high" : py.storageForecast.risk,
                                    actionLabel: (over50 || py.storageForecast.risk != "low") ? "Scan Caches" : nil,
                                    actionPrompt: "/caches"
                                )

                                // 3. Abnormal App Behaviour (IsolationForest)
                                if let apps = py.abnormalApps, !apps.isEmpty {
                                    ForEach(apps.prefix(2)) { app in
                                        forecastRow(
                                            icon: "exclamationmark.triangle.fill",
                                            title: "Abnormal App: \(app.name)",
                                            detail: "\(app.reason) · Score \(String(format: "%.2f", app.anomalyScore))",
                                            risk: "high",
                                            actionLabel: "Inspect",
                                            actionPrompt: "Inspect process \(app.name) (PID \(app.pid))"
                                        )
                                    }
                                } else {
                                    forecastRow(
                                        icon: "checkmark.shield.fill",
                                        title: "App Behaviour (IsolationForest)",
                                        detail: "No abnormal CPU/memory outliers across active apps.",
                                        risk: "low",
                                        actionLabel: nil,
                                        actionPrompt: ""
                                    )
                                }
                            }
                        } else {
                            Text("Training Ridge & IsolationForest baseline on live telemetry...")
                                .font(.system(size: 11, design: .monospaced))
                                .foregroundStyle(HUDColors.textMuted)
                        }
                    }
                    Spacer(minLength: 0)
                }
                .frame(minWidth: 280, idealWidth: 320, maxWidth: 360)

                // Center: Animated Core + Live Hardware Chips
                VStack(spacing: 16) {
                    Spacer()
                    SyntrixCoreView(
                        cpuPct: client.snapshot?.cpu.totalUsagePct ?? 0,
                        memoryPressurePct: client.snapshot?.memory.pressurePct ?? 0,
                        mode: client.mode,
                        isConnected: client.isConnected
                    )
                    if let snap = client.snapshot {
                        HStack(spacing: 10) {
                            metricChip(label: "NET IN", value: HUDFormatters.rate(snap.network.bytesInPerSec))
                            metricChip(label: "NET OUT", value: HUDFormatters.rate(snap.network.bytesOutPerSec))
                            metricChip(label: "SWAP", value: HUDFormatters.bytesMB(snap.memory.swapUsedBytes))
                            metricChip(label: "THERMAL", value: (snap.thermal?.stateLabel ?? "nominal").uppercased())
                        }
                    }
                    Spacer()
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)

                // Right column: Guardian Agent + Pending Optimizations + Recent Activity
                VStack(spacing: 14) {
                    if let g = client.guardianStatus {
                        let gColor: Color = g.state == "PAUSED_KILL_SWITCH" ? HUDColors.red
                            : (g.state == "ACTING" || g.state == "WATCHING_AWAY" ? HUDColors.amber
                            : (g.enabled ? HUDColors.green : HUDColors.textMuted))
                        HUDPanel(title: "GUARDIAN AGENT", trailing: (g.devBoostActive ?? false) ? "DEV BOOST · \(g.state)" : g.state) {
                            VStack(alignment: .leading, spacing: 6) {
                                HStack(spacing: 8) {
                                    Text("ABSENCE MONITOR")
                                        .font(.system(size: 9, weight: .bold, design: .monospaced))
                                        .foregroundStyle(HUDColors.textMuted)
                                    Text("AWAY \(g.idleSeconds)s / \(g.idleThresholdSec)s")
                                        .font(.system(size: 9, design: .monospaced))
                                        .foregroundStyle(gColor)
                                    Spacer()
                                    ForEach([120, 180, 300], id: \.self) { sec in
                                        Button("\(sec / 60)m") {
                                            client.setGuardian(enabled: g.enabled, idleThresholdSec: sec)
                                        }
                                        .font(.system(size: 8, weight: .bold, design: .monospaced))
                                        .foregroundStyle(g.idleThresholdSec == sec ? HUDColors.cyan : HUDColors.textMuted)
                                        .padding(.horizontal, 5)
                                        .padding(.vertical, 2)
                                        .overlay(RoundedRectangle(cornerRadius: 2).stroke(g.idleThresholdSec == sec ? HUDColors.cyan : HUDColors.cyanDim, lineWidth: 1))
                                        .buttonStyle(.plain)
                                    }
                                    Button(g.enabled ? "ON" : "OFF") {
                                        client.setGuardian(enabled: !g.enabled, idleThresholdSec: g.idleThresholdSec)
                                    }
                                    .font(.system(size: 9, weight: .bold, design: .monospaced))
                                    .foregroundStyle(g.enabled ? HUDColors.green : HUDColors.textMuted)
                                    .padding(.horizontal, 7)
                                    .padding(.vertical, 2)
                                    .background((g.enabled ? HUDColors.green : HUDColors.textMuted).opacity(0.14))
                                    .overlay(RoundedRectangle(cornerRadius: 2).stroke(g.enabled ? HUDColors.green : HUDColors.cyanDim, lineWidth: 1))
                                    .buttonStyle(.plain)
                                }

                                HStack(spacing: 12) {
                                    Text("Fulfilled: \(g.actionsTaken)")
                                        .foregroundStyle(HUDColors.green)
                                    Text("Prevented: \(g.proactivePrevented ?? 0)")
                                        .foregroundStyle(HUDColors.cyan)
                                    Text("RL Reward: \(String(format: "%+.2f", g.rlAvgReward ?? 0.0))")
                                        .foregroundStyle(HUDColors.amber)
                                    Spacer()
                                    Text(g.lastAction)
                                        .foregroundStyle(HUDColors.textMuted)
                                        .lineLimit(1)
                                }
                                .font(.system(size: 9, design: .monospaced))
                            }
                        }
                    }

                    let pendingRecs = client.recommendations.filter { $0.status == "pending" }
                    let lastApproved = client.recommendations.first { $0.status == "approved" }
                    HUDPanel(title: "OPTIMIZATIONS", trailing: "\(pendingRecs.count) PENDING") {
                        HStack {
                            Spacer()
                            Button {
                                client.activateKillSwitch()
                            } label: {
                                Label("STOP ALL", systemImage: "stop.circle.fill")
                                    .font(.system(size: 9, weight: .bold, design: .monospaced))
                                    .foregroundStyle(HUDColors.red)
                                    .padding(.horizontal, 7)
                                    .padding(.vertical, 2)
                                    .background(HUDColors.red.opacity(0.15))
                                    .cornerRadius(3)
                            }
                            .buttonStyle(.plain)
                            .help("Kill switch: immediately abort all running/pending optimizations")
                        }
                        .padding(.top, -4)
                        let recs = pendingRecs
                        if recs.isEmpty {
                            VStack(alignment: .leading, spacing: 6) {
                                HStack(spacing: 8) {
                                    Image(systemName: "checkmark.seal.fill")
                                        .font(.system(size: 11))
                                        .imageScale(.small)
                                        .foregroundStyle(HUDColors.green)
                                    Text("All systems optimal — no pending actions.")
                                        .font(.system(size: 11, design: .monospaced))
                                        .foregroundStyle(HUDColors.textMuted)
                                }
                                if let last = lastApproved, !last.afterMetric.isEmpty {
                                    Text("Last applied: \(last.title) (\(last.afterMetric))")
                                        .font(.system(size: 9, design: .monospaced))
                                        .foregroundStyle(HUDColors.green)
                                }
                            }
                            .padding(.vertical, 6)
                        } else {
                            ScrollView {
                                VStack(alignment: .leading, spacing: 8) {
                                    ForEach(recs) { rec in
                                        VStack(alignment: .leading, spacing: 4) {
                                            HStack {
                                                Text(rec.title)
                                                    .font(.system(size: 11, weight: .semibold, design: .monospaced))
                                                    .foregroundStyle(rec.severity == "high" ? HUDColors.amber : HUDColors.textPrimary)
                                                    .lineLimit(1)
                                                Spacer()
                                                Text(rec.status.uppercased())
                                                    .font(.system(size: 9, weight: .bold, design: .monospaced))
                                                    .foregroundStyle(rec.status == "approved" ? HUDColors.green : HUDColors.cyan)
                                            }
                                            Text(rec.detail)
                                                .font(.system(size: 10, design: .monospaced))
                                                .foregroundStyle(HUDColors.textMuted)
                                                .lineLimit(2)

                                            if !rec.afterMetric.isEmpty {
                                                Text("OUTCOME: \(rec.afterMetric)")
                                                    .font(.system(size: 9, weight: .semibold, design: .monospaced))
                                                    .foregroundStyle(HUDColors.green)
                                            }

                                            if rec.status == "pending" {
                                                HStack(spacing: 8) {
                                                    Button {
                                                        client.previewRecommendation(rec.id)
                                                        client.activeTab = AppScreen.assistant.rawValue
                                                    } label: {
                                                        Label("Preview", systemImage: "eye")
                                                            .font(.system(size: 10, design: .monospaced))
                                                            .foregroundStyle(HUDColors.cyan)
                                                            .padding(.horizontal, 8)
                                                            .padding(.vertical, 3)
                                                            .background(HUDColors.cyan.opacity(0.12))
                                                            .cornerRadius(3)
                                                    }
                                                    .buttonStyle(.plain)

                                                    Button {
                                                        client.approveRecommendation(rec.id)
                                                    } label: {
                                                        Label("Approve", systemImage: "checkmark.circle.fill")
                                                            .font(.system(size: 10, weight: .semibold, design: .monospaced))
                                                            .foregroundStyle(HUDColors.green)
                                                            .padding(.horizontal, 8)
                                                            .padding(.vertical, 3)
                                                            .background(HUDColors.green.opacity(0.15))
                                                            .cornerRadius(3)
                                                    }
                                                    .buttonStyle(.plain)

                                                    Button {
                                                        client.dismissRecommendation(rec.id)
                                                    } label: {
                                                        Label("Dismiss", systemImage: "xmark.circle")
                                                            .font(.system(size: 10, design: .monospaced))
                                                            .foregroundStyle(HUDColors.textMuted)
                                                            .padding(.horizontal, 8)
                                                            .padding(.vertical, 3)
                                                            .background(HUDColors.cyanDim.opacity(0.15))
                                                            .cornerRadius(3)
                                                    }
                                                    .buttonStyle(.plain)
                                                }
                                            }
                                        }
                                        .padding(8)
                                        .background(HUDColors.cyanDim.opacity(0.12))
                                        .cornerRadius(4)
                                    }
                                }
                            }
                            .frame(maxHeight: 240)
                        }
                    }

                    HUDPanel(title: "TOP PROCESSES", trailing: "CLICK TO INSPECT") {
                        let procs = Array((client.snapshot?.topProcesses ?? []).prefix(5))
                        if procs.isEmpty {
                            Text("Awaiting process telemetry...")
                                .font(.system(size: 11, design: .monospaced))
                                .foregroundStyle(HUDColors.textMuted)
                        } else {
                            VStack(spacing: 5) {
                                ForEach(procs) { proc in
                                    Button {
                                        client.sendPrompt("Inspect process \(proc.name) (PID \(proc.pid))")
                                        client.activeTab = AppScreen.assistant.rawValue
                                    } label: {
                                        HStack {
                                            Text(proc.name)
                                                .font(.system(size: 11, weight: .medium, design: .monospaced))
                                                .foregroundStyle(HUDColors.textPrimary)
                                                .lineLimit(1)
                                            Spacer()
                                            Text(String(format: "%.1f%%", proc.cpuPct))
                                                .font(.system(size: 10, design: .monospaced))
                                                .foregroundStyle(proc.cpuPct > 40 ? HUDColors.amber : HUDColors.cyan)
                                                .frame(width: 46, alignment: .trailing)
                                            Text(HUDFormatters.bytesMB(proc.memoryBytes))
                                                .font(.system(size: 10, design: .monospaced))
                                                .foregroundStyle(HUDColors.textMuted)
                                                .frame(width: 54, alignment: .trailing)
                                        }
                                        .padding(.vertical, 2)
                                        .contentShape(Rectangle())
                                    }
                                    .buttonStyle(.plain)
                                }
                            }
                        }
                    }
                    Spacer(minLength: 0)
                }
                .frame(minWidth: 280, idealWidth: 330, maxWidth: 370)
            }
            .frame(maxHeight: .infinity)

            QuickCommandBar(client: client, switchToAssistantOnSend: true)
        }
    }

    private func interactiveHealthBar(key: String, valuePct: Double, accent: Color) -> some View {
        let selected = client.selectedHealthMetric == key
        return Button {
            client.selectedHealthMetric = key
        } label: {
            HStack(spacing: 6) {
                HUDProgressBar(label: key, valuePct: valuePct, accent: accent)
                Image(systemName: selected ? "chevron.down" : "chevron.right")
                    .font(.system(size: 9))
                    .foregroundStyle(selected ? HUDColors.cyan : HUDColors.textMuted)
            }
            .padding(.horizontal, 6)
            .padding(.vertical, 3)
            .background(selected ? HUDColors.cyan.opacity(0.10) : Color.clear)
            .cornerRadius(4)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
    }

    @ViewBuilder
    private var healthInsightCard: some View {
        if let snap = client.snapshot {
            let key = client.selectedHealthMetric
            let insight: (title: String, detail: String, rec: String, btn: String, cmd: String) = {
                switch key {
                case "CPU":
                    let topCPU = snap.topProcesses.max(by: { $0.cpuPct < $1.cpuPct })
                    let topText = topCPU.map { "\($0.name) (\($0.cpuPct.formatted(.number.precision(.fractionLength(1))))%)" } ?? "Idle"
                    let thermal = (snap.thermal?.stateLabel ?? "nominal").uppercased()
                    let rec = snap.cpu.totalUsagePct > 70 ? "High CPU load — inspect top processes or throttle background builds." : "CPU headroom is healthy across \(snap.cpu.coreCount) cores."
                    return ("CPU INSIGHT · \(snap.cpu.coreCount) CORES", "Top: \(topText) · Thermal: \(thermal) (\(Int(snap.thermal?.cpuSpeedLimitPct ?? 100))%)", rec, "Inspect Top Apps", "/top")
                case "DSK":
                    let fc = client.storageAudit?.summary ?? client.storageForecast?.summary ?? "Baseline collecting"
                    let topUnused = (client.storageAudit?.candidates ?? []).prefix(2).map { "\($0.owner) (\(Int($0.sizeMb))MB)" }.joined(separator: ", ")
                    let rec = snap.disk.usedPct > 50
                        ? (topUnused.isEmpty ? "Exceeded 50% limit — scan caches & Downloads." : "Unused: \(topUnused)")
                        : "Within 50% storage ceiling."
                    return ("DISK INSIGHT · \(Int(snap.disk.usedPct))% / 50% MAX", fc, rec, "Scan Caches", "/caches")
                case "BAT":
                    let src = snap.power.onACPower ? "AC Power" : "Battery"
                    let chg = snap.power.isCharging ? "Charging" : (snap.power.onACPower ? "Plugged In" : "Discharging")
                    let rec = snap.power.onACPower ? "Connected to AC — full performance available." : "On battery — keep background indexing light."
                    return ("POWER INSIGHT · \(Int(snap.power.batteryPct))%", "\(src) · \(chg)", rec, "Power Report", "battery status")
                default:
                    let usedGB = Double(snap.memory.usedBytes) / (1024 * 1024 * 1024)
                    let totalGB = Double(snap.memory.totalBytes) / (1024 * 1024 * 1024)
                    let rec = snap.memory.pressurePct > 70 ? "Elevated memory pressure — free RAM or close heavy tabs." : "Memory pressure is within comfortable bounds."
                    return (
                        String(format: "MEMORY INSIGHT · %.2f / %.1f GB", usedGB, totalGB),
                        "Compressed \(HUDFormatters.bytesMB(snap.memory.compressedBytes)) · Swap \(HUDFormatters.bytesMB(snap.memory.swapUsedBytes)) · \(snap.memory.pressureLevel.uppercased())",
                        rec,
                        "Optimize RAM",
                        "/optimize-memory"
                    )
                }
            }()

            VStack(alignment: .leading, spacing: 5) {
                HStack {
                    Text(insight.title)
                        .font(.system(size: 9, weight: .bold, design: .monospaced))
                        .foregroundStyle(HUDColors.cyan)
                    Spacer()
                    Button(insight.btn) {
                        client.sendPrompt(insight.cmd)
                        client.activeTab = AppScreen.assistant.rawValue
                    }
                    .font(.system(size: 9, weight: .bold, design: .monospaced))
                    .foregroundStyle(HUDColors.amber)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(HUDColors.amber.opacity(0.15))
                    .cornerRadius(3)
                    .buttonStyle(.plain)
                }
                Text(insight.detail)
                    .font(.system(size: 9, design: .monospaced))
                    .foregroundStyle(HUDColors.textPrimary)
                Text(insight.rec)
                    .font(.system(size: 9, design: .monospaced))
                    .foregroundStyle(HUDColors.textMuted)
            }
            .padding(8)
            .background(HUDColors.cyanDim.opacity(0.12))
            .cornerRadius(4)
        }
    }

    private func forecastRow(icon: String, title: String, detail: String, risk: String, actionLabel: String?, actionPrompt: String) -> some View {
        let riskColor: Color = risk == "high" ? HUDColors.red : (risk == "moderate" ? HUDColors.amber : HUDColors.green)
        return VStack(alignment: .leading, spacing: 3) {
            HStack(spacing: 6) {
                Image(systemName: icon)
                    .font(.system(size: 10))
                    .foregroundStyle(riskColor)
                Text(title)
                    .font(.system(size: 10, weight: .bold, design: .monospaced))
                    .foregroundStyle(HUDColors.textPrimary)
                Spacer()
                Text(risk.uppercased())
                    .font(.system(size: 8, weight: .bold, design: .monospaced))
                    .padding(.horizontal, 5)
                    .padding(.vertical, 1)
                    .background(riskColor.opacity(0.18))
                    .foregroundStyle(riskColor)
                    .cornerRadius(3)
            }
            HStack {
                Text(detail)
                    .font(.system(size: 9, design: .monospaced))
                    .foregroundStyle(HUDColors.textMuted)
                    .fixedSize(horizontal: false, vertical: true)
                Spacer()
                if let label = actionLabel {
                    Button(label) {
                        client.sendPrompt(actionPrompt)
                        client.activeTab = AppScreen.assistant.rawValue
                    }
                    .font(.system(size: 9, weight: .bold, design: .monospaced))
                    .foregroundStyle(HUDColors.amber)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(HUDColors.amber.opacity(0.16))
                    .cornerRadius(3)
                    .buttonStyle(.plain)
                }
            }
        }
        .padding(6)
        .background(HUDColors.cyanDim.opacity(0.10))
        .cornerRadius(4)
    }

    private func metricChip(label: String, value: String) -> some View {
        VStack(spacing: 2) {
            Text(label)
                .font(.system(size: 9, weight: .medium, design: .monospaced))
                .foregroundStyle(HUDColors.textMuted)
            Text(value)
                .font(.system(size: 11, weight: .semibold, design: .monospaced))
                .foregroundStyle(HUDColors.cyan)
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
        .background(HUDColors.panelBg)
        .overlay(
            RoundedRectangle(cornerRadius: 4)
                .stroke(HUDColors.cyanDim, lineWidth: 1)
        )
    }
}

struct QuickCommandBar: View {
    @ObservedObject var client: SyntrixClient
    var switchToAssistantOnSend: Bool = false

    private let quickActions: [(label: String, cmd: String, icon: String, prefill: Bool)] = [
        ("Diagnose", "/diagnose", "waveform.path.ecg", false),
        ("Optimize RAM", "/optimize-memory", "memorychip", false),
        ("System Health", "/status", "heart.text.square", false),
        ("Top Apps", "/top", "list.number", false),
        ("Disk Forecast", "/storage-forecast", "chart.line.uptrend.xyaxis", false),
        ("Build Plan", "/plan ", "checklist", true),
        ("Audit Log", "/audit", "shield.checkered", false)
    ]

    private let slashCommands: [(cmd: String, desc: String, needsArg: Bool)] = [
        ("/status", "Live CPU, memory, thermal & disk summary", false),
        ("/diagnose", "Causal root-cause diagnosis & alternatives", false),
        ("/optimize-memory", "Analyze RAM pressure & propose safe actions", false),
        ("/top", "List top resource-consuming processes", false),
        ("/scan", "Run full optimization scan", false),
        ("/caches", "Inspect stale user & app cache directories", false),
        ("/storage-forecast", "Predict storage growth & exhaustion window", false),
        ("/plan ", "Build a risk-classified desktop agent plan", true),
        ("/startup", "Inspect LaunchAgents & startup items", false),
        ("/dev ", "Inspect a local project directory", true),
        ("/learn ", "Build an offline learning path on a topic", true),
        ("/audit", "Show recent SQLite audit trail", false),
        ("/help", "List all supported commands", false)
    ]

    private var matchingCommands: [(cmd: String, desc: String, needsArg: Bool)] {
        let trimmed = client.inputText.trimmingCharacters(in: .whitespaces).lowercased()
        guard trimmed.hasPrefix("/") else { return [] }
        return slashCommands.filter { $0.cmd.lowercased().hasPrefix(trimmed) || $0.cmd.lowercased().contains(trimmed) }.prefix(5).map { $0 }
    }

    var body: some View {
        VStack(spacing: 6) {
            if !matchingCommands.isEmpty {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 6) {
                        ForEach(matchingCommands, id: \.cmd) { item in
                            Button {
                                if item.needsArg {
                                    client.inputText = item.cmd
                                } else {
                                    client.inputText = ""
                                    client.sendPrompt(item.cmd)
                                    if switchToAssistantOnSend {
                                        client.activeTab = AppScreen.assistant.rawValue
                                    }
                                }
                            } label: {
                                HStack(spacing: 5) {
                                    Text(item.cmd)
                                        .font(.system(size: 10, weight: .bold, design: .monospaced))
                                        .foregroundStyle(HUDColors.amber)
                                    Text("· \(item.desc)")
                                        .font(.system(size: 9, design: .monospaced))
                                        .foregroundStyle(HUDColors.textMuted)
                                }
                                .padding(.horizontal, 8)
                                .padding(.vertical, 4)
                                .background(HUDColors.panelBg)
                                .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.amber.opacity(0.45), lineWidth: 1))
                            }
                            .buttonStyle(.plain)
                        }
                    }
                }
            }

            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 8) {
                    ForEach(quickActions, id: \.cmd) { qa in
                        Button {
                            if qa.prefill {
                                client.inputText = qa.cmd
                            } else {
                                client.sendPrompt(qa.cmd)
                                if switchToAssistantOnSend {
                                    client.activeTab = AppScreen.assistant.rawValue
                                }
                            }
                        } label: {
                            HStack(spacing: 4) {
                                Image(systemName: qa.icon)
                                    .font(.system(size: 9))
                                Text(qa.label)
                                    .font(.system(size: 10, weight: .medium, design: .monospaced))
                            }
                            .padding(.horizontal, 8)
                            .padding(.vertical, 4)
                            .background(HUDColors.panelBg)
                            .foregroundStyle(qa.prefill ? HUDColors.amber : HUDColors.cyan)
                            .overlay(
                                RoundedRectangle(cornerRadius: 3)
                                    .stroke((qa.prefill ? HUDColors.amber : HUDColors.cyanDim), lineWidth: 1)
                            )
                        }
                        .buttonStyle(.plain)
                        .help(qa.prefill ? "Pre-fill \(qa.cmd)" : "Runs \(qa.cmd)")
                    }
                }
            }

            HStack(spacing: 10) {
                Text(">")
                    .font(.system(size: 13, weight: .bold, design: .monospaced))
                    .foregroundStyle(HUDColors.cyan)

                TextField("Ask SYNTRIX anything (type / for commands, or 'Why is my Mac slow?')...", text: $client.inputText)
                    .textFieldStyle(.plain)
                    .font(.system(size: 12, design: .monospaced))
                    .foregroundStyle(HUDColors.textPrimary)
                    .onSubmit {
                        submit()
                    }

                Button {
                    submit()
                } label: {
                    HStack(spacing: 5) {
                        if client.isAssistantThinking {
                            ProgressView()
                                .scaleEffect(0.55)
                                .frame(width: 10, height: 10)
                            Text("THINKING")
                        } else {
                            Image(systemName: "paperplane.fill")
                                .font(.system(size: 10))
                                .imageScale(.small)
                            Text("SEND")
                        }
                    }
                    .font(.system(size: 10, weight: .semibold, design: .monospaced))
                    .foregroundStyle(client.isAssistantThinking ? HUDColors.amber : HUDColors.cyan)
                    .padding(.horizontal, 10)
                    .padding(.vertical, 5)
                    .overlay(
                        RoundedRectangle(cornerRadius: 3)
                            .stroke(client.isAssistantThinking ? HUDColors.amber : HUDColors.cyan, lineWidth: 1)
                    )
                }
                .buttonStyle(.plain)
                .disabled(client.isAssistantThinking)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
            .background(HUDColors.panelBg)
            .overlay(
                RoundedRectangle(cornerRadius: 4)
                    .stroke(HUDColors.cyanDim, lineWidth: 1)
            )
            .overlay(HUDCorners())
        }
    }

    private func submit() {
        let cmd = client.inputText
        guard !cmd.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return }
        client.inputText = ""
        client.sendPrompt(cmd)
        if switchToAssistantOnSend {
            client.activeTab = AppScreen.assistant.rawValue
        }
    }
}
