import SwiftUI

struct TelemetryScreen: View {
    @ObservedObject var client: SyntrixClient

    private var filteredAndSortedProcesses: [ProcessInfoItem] {
        let procs = client.snapshot?.topProcesses ?? []
        let query = client.processSearch.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        let filtered = query.isEmpty ? procs : procs.filter {
            $0.name.lowercased().contains(query) || "\($0.pid)".contains(query)
        }
        return filtered.sorted { a, b in
            let asc = client.processSortAscending
            switch client.processSortKey {
            case "CPU":
                return asc ? a.cpuPct < b.cpuPct : a.cpuPct > b.cpuPct
            case "THREADS":
                return asc ? a.threads < b.threads : a.threads > b.threads
            case "PID":
                return asc ? a.pid < b.pid : a.pid > b.pid
            case "NAME":
                return asc ? a.name.localizedCaseInsensitiveCompare(b.name) == .orderedAscending
                           : a.name.localizedCaseInsensitiveCompare(b.name) == .orderedDescending
            default: // "RSS"
                return asc ? a.memoryBytes < b.memoryBytes : a.memoryBytes > b.memoryBytes
            }
        }
    }

    var body: some View {
        ScrollView {
            VStack(spacing: 12) {
                telemetryControlBar

                if let snap = client.snapshot {
                    sparklineRow(snap)
                }

                HStack(alignment: .top, spacing: 14) {
                    hardwareAndMemoryColumn
                    intelligenceAndGovernorColumn
                }

                processInspectorPanel
            }
        }
    }

    private var telemetryControlBar: some View {
        HStack(spacing: 10) {
            Button {
                client.isTelemetryPaused.toggle()
            } label: {
                HStack(spacing: 5) {
                    Image(systemName: client.isTelemetryPaused ? "play.fill" : "pause.fill")
                        .font(.system(size: 9))
                    Text(client.isTelemetryPaused ? "STREAM PAUSED (RESUME)" : "LIVE TELEMETRY")
                        .font(.system(size: 10, weight: .bold, design: .monospaced))
                }
                .padding(.horizontal, 9)
                .padding(.vertical, 5)
                .background((client.isTelemetryPaused ? HUDColors.amber : HUDColors.green).opacity(0.14))
                .foregroundStyle(client.isTelemetryPaused ? HUDColors.amber : HUDColors.green)
                .overlay(
                    RoundedRectangle(cornerRadius: 3)
                        .stroke(client.isTelemetryPaused ? HUDColors.amber : HUDColors.green, lineWidth: 1)
                )
            }
            .buttonStyle(.plain)

            if let snap = client.snapshot {
                Text("\(snap.cpu.coreCount) CORES · \(snap.processCountTotal) PROCESSES · THERMAL \((snap.thermal?.stateLabel ?? "nominal").uppercased())")
                    .font(.system(size: 10, design: .monospaced))
                    .foregroundStyle(HUDColors.textMuted)
            }

            Spacer()

            Button {
                client.sendPrompt("/diagnose")
                client.activeTab = AppScreen.assistant.rawValue
            } label: {
                Label("Causal Diagnose", systemImage: "waveform.path.ecg")
                    .font(.system(size: 10, weight: .semibold, design: .monospaced))
                    .foregroundStyle(HUDColors.cyan)
                    .padding(.horizontal, 8)
                    .padding(.vertical, 4)
                    .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.cyan, lineWidth: 1))
            }
            .buttonStyle(.plain)

            Button {
                client.sendPrompt("/optimize-memory")
                client.activeTab = AppScreen.assistant.rawValue
            } label: {
                Label("Optimize RAM", systemImage: "memorychip")
                    .font(.system(size: 10, weight: .semibold, design: .monospaced))
                    .foregroundStyle(HUDColors.amber)
                    .padding(.horizontal, 8)
                    .padding(.vertical, 4)
                    .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.amber, lineWidth: 1))
            }
            .buttonStyle(.plain)
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 7)
        .background(HUDColors.panelBg)
        .overlay(RoundedRectangle(cornerRadius: 4).stroke(HUDColors.cyanDim, lineWidth: 1))
    }

    private func sparklineRow(_ snap: SystemSnapshot) -> some View {
        HStack(spacing: 12) {
            sparklineCard(
                title: "CPU UTILIZATION",
                value: String(format: "%.1f%%", snap.cpu.totalUsagePct),
                subtitle: String(format: "User %.0f%% · Sys %.0f%%", snap.cpu.userPct, snap.cpu.systemPct),
                samples: client.cpuHistory,
                maxScale: 100.0,
                color: snap.cpu.totalUsagePct > 75 ? HUDColors.red : HUDColors.cyan
            )

            sparklineCard(
                title: "MEMORY PRESSURE",
                value: String(format: "%.0f%%", snap.memory.pressurePct),
                subtitle: "\(snap.memory.pressureLevel.uppercased()) · Swap \(HUDFormatters.bytesMB(snap.memory.swapUsedBytes))",
                samples: client.memPressureHistory,
                maxScale: 100.0,
                color: snap.memory.pressurePct > 75 ? HUDColors.red : (snap.memory.pressurePct > 55 ? HUDColors.amber : HUDColors.green)
            )

            let maxNet = max(10.0, (client.netInHistory + client.netOutHistory).max() ?? 10.0)
            sparklineCard(
                title: "NETWORK I/O",
                value: "↓ \(HUDFormatters.rate(snap.network.bytesInPerSec))",
                subtitle: "↑ \(HUDFormatters.rate(snap.network.bytesOutPerSec))",
                samples: client.netInHistory,
                maxScale: maxNet,
                color: HUDColors.cyan
            )
        }
    }

    private func sparklineCard(title: String, value: String, subtitle: String, samples: [Double], maxScale: Double, color: Color) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text(title)
                    .font(.system(size: 9, weight: .bold, design: .monospaced))
                    .foregroundStyle(HUDColors.textMuted)
                Spacer()
                Text(value)
                    .font(.system(size: 12, weight: .bold, design: .monospaced))
                    .foregroundStyle(color)
            }

            SparklineGraph(samples: samples, maxScale: maxScale, color: color)
                .frame(height: 36)

            Text(subtitle)
                .font(.system(size: 9, design: .monospaced))
                .foregroundStyle(HUDColors.textMuted)
                .lineLimit(1)
        }
        .padding(10)
        .background(HUDColors.panelBg)
        .overlay(RoundedRectangle(cornerRadius: 4).stroke(color.opacity(0.35), lineWidth: 1))
        .overlay(HUDCorners())
        .frame(maxWidth: .infinity)
    }

    private var hardwareAndMemoryColumn: some View {
        VStack(spacing: 14) {
            HUDPanel(title: "HARDWARE & PER-CORE CPU", trailing: client.snapshot.map { "\($0.cpu.coreCount) CORES" }) {
                if let snap = client.snapshot {
                    VStack(alignment: .leading, spacing: 10) {
                        HUDProgressBar(label: "CPU", valuePct: snap.cpu.totalUsagePct, accent: snap.cpu.totalUsagePct > 75 ? HUDColors.red : HUDColors.amber)
                        HUDProgressBar(label: "MEM", valuePct: snap.memory.pressurePct, accent: snap.memory.pressurePct > 75 ? HUDColors.red : HUDColors.cyan)
                        HUDProgressBar(label: "DSK", valuePct: snap.disk.usedPct, accent: snap.disk.usedPct > 75 ? HUDColors.red : (snap.disk.usedPct > 50 ? HUDColors.amber : HUDColors.cyan))

                        if !snap.cpu.perCorePct.isEmpty {
                            Divider().overlay(HUDColors.cyanDim)
                            Text("PER-CORE UTILIZATION")
                                .font(.system(size: 9, weight: .bold, design: .monospaced))
                                .foregroundStyle(HUDColors.textMuted)

                            LazyVGrid(columns: [GridItem(.flexible()), GridItem(.flexible())], spacing: 6) {
                                ForEach(Array(snap.cpu.perCorePct.enumerated()), id: \.offset) { idx, pct in
                                    let coreColor = pct > 80 ? HUDColors.red : (pct > 50 ? HUDColors.amber : HUDColors.cyan)
                                    HStack(spacing: 6) {
                                        Text("C\(idx)")
                                            .font(.system(size: 9, weight: .bold, design: .monospaced))
                                            .foregroundStyle(HUDColors.textMuted)
                                            .frame(width: 22, alignment: .leading)
                                        GeometryReader { geo in
                                            ZStack(alignment: .leading) {
                                                RoundedRectangle(cornerRadius: 2)
                                                    .fill(HUDColors.cyanDim.opacity(0.3))
                                                RoundedRectangle(cornerRadius: 2)
                                                    .fill(coreColor)
                                                    .frame(width: max(2, geo.size.width * min(1.0, pct / 100.0)))
                                            }
                                        }
                                        .frame(height: 6)
                                        Text(String(format: "%3.0f%%", pct))
                                            .font(.system(size: 9, design: .monospaced))
                                            .foregroundStyle(coreColor)
                                            .frame(width: 30, alignment: .trailing)
                                    }
                                }
                            }
                        }

                        Divider().overlay(HUDColors.cyanDim)
                        memoryCompositionBar(snap.memory)

                        statRow("Memory Used / Total", "\(HUDFormatters.bytesGB(snap.memory.usedBytes)) / \(HUDFormatters.bytesGB(snap.memory.totalBytes))")
                        statRow("Compressed / Swap", "\(HUDFormatters.bytesMB(snap.memory.compressedBytes)) / \(HUDFormatters.bytesMB(snap.memory.swapUsedBytes))")
                        statRow("Thermal Speed Limit", "\((snap.thermal?.stateLabel ?? "nominal").uppercased()) (\(Int(snap.thermal?.cpuSpeedLimitPct ?? 100))%)")
                        statRow("Power Source", snap.power.hasBattery ? "\(Int(snap.power.batteryPct))% (\(snap.power.isCharging ? "Charging" : "Battery"))" : "AC Power")
                    }
                } else {
                    Text("Waiting for C17 telemetry snapshot...")
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundStyle(HUDColors.textMuted)
                }
            }
        }
    }

    private func memoryCompositionBar(_ mem: MemoryMetrics) -> some View {
        let total = max(1.0, Double(mem.totalBytes))
        let activeFrac = Double(mem.activeBytes) / total
        let wiredFrac = Double(mem.wiredBytes) / total
        let compFrac = Double(mem.compressedBytes) / total

        return VStack(alignment: .leading, spacing: 5) {
            Text("MEMORY COMPOSITION")
                .font(.system(size: 9, weight: .bold, design: .monospaced))
                .foregroundStyle(HUDColors.textMuted)

            GeometryReader { geo in
                HStack(spacing: 1) {
                    Rectangle().fill(HUDColors.cyan).frame(width: max(0, geo.size.width * activeFrac))
                    Rectangle().fill(HUDColors.amber).frame(width: max(0, geo.size.width * wiredFrac))
                    Rectangle().fill(HUDColors.red).frame(width: max(0, geo.size.width * compFrac))
                    Rectangle().fill(HUDColors.green.opacity(0.25))
                }
                .clipShape(RoundedRectangle(cornerRadius: 3))
            }
            .frame(height: 8)

            HStack(spacing: 10) {
                legendDot("Active", HUDColors.cyan)
                legendDot("Wired", HUDColors.amber)
                legendDot("Compressed", HUDColors.red)
                legendDot("Free/Inactive", HUDColors.green.opacity(0.6))
            }
        }
    }

    private func legendDot(_ label: String, _ color: Color) -> some View {
        HStack(spacing: 4) {
            Circle().fill(color).frame(width: 6, height: 6)
            Text(label)
                .font(.system(size: 8, design: .monospaced))
                .foregroundStyle(HUDColors.textMuted)
        }
    }

    private var intelligenceAndGovernorColumn: some View {
        VStack(spacing: 14) {
            HUDPanel(title: "RESOURCE GOVERNOR", trailing: client.governorStatus?.state ?? "INIT") {
                if let gov = client.governorStatus {
                    VStack(alignment: .leading, spacing: 6) {
                        HStack(spacing: 8) {
                            statusPill("STATE: \(gov.state)", color: gov.state == "NORMAL" ? HUDColors.green : HUDColors.amber)
                            statusPill(gov.allowIndexing ? "INDEXING: ON" : "INDEXING: PAUSED", color: gov.allowIndexing ? HUDColors.green : HUDColors.amber)
                            statusPill(gov.allowInference ? "ML: ON" : "ML: SUSPENDED", color: gov.allowInference ? HUDColors.green : HUDColors.red)
                        }
                        statRow("Host Mem Pressure", String(format: "%.1f%%", gov.memoryPressurePct))
                        statRow("Daemon Heap / Goroutines", String(format: "%.2f MB / %d", gov.goHeapAllocMB, gov.goroutines))
                        statRow("Max Concurrent Agents", "\(gov.maxConcurrentAgents)")
                    }
                } else {
                    Text("Governor status pending...")
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundStyle(HUDColors.textMuted)
                }
            }

            HUDPanel(title: "DAEMON SELF-HEALTH", trailing: "OBSERVABILITY") {
                if let sh = client.selfHealth {
                    VStack(alignment: .leading, spacing: 6) {
                        statRow("C17 Collector Latency", "\(sh.collectorLatencyUs) µs")
                        statRow("SQLite Write Latency", "\(sh.sqliteWriteUs) µs")
                        statRow("Journaled Events", "\(sh.eventCount)")
                        statRow("Heap / Goroutines", String(format: "%.2f MB / %d", sh.heapAllocMB, sh.goroutines))
                    }
                } else {
                    Text("Self-health metrics pending...")
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundStyle(HUDColors.textMuted)
                }
            }

            HUDPanel(title: "PYTHON ML FORECAST", trailing: client.pythonForecast.map { "\($0.sampleCount) SAMPLES" }) {
                if let py = client.pythonForecast {
                    VStack(alignment: .leading, spacing: 7) {
                        HStack(spacing: 6) {
                            statusPill(py.memoryForecast.selectedModel ?? py.modelBackend, color: HUDColors.cyan)
                            if let ver = py.modelVersion {
                                statusPill("v\(ver)", color: HUDColors.textMuted)
                            }
                            if let drift = py.memoryForecast.driftDetected {
                                statusPill(drift ? "DRIFT DETECTED" : "MODEL STABLE", color: drift ? HUDColors.amber : HUDColors.green)
                            }
                        }

                        HStack(spacing: 6) {
                            horizonPill("1m", py.memoryForecast.predicted1mPct ?? py.memoryForecast.currentPct)
                            horizonPill("5m", py.memoryForecast.predicted5mPct)
                            horizonPill("15m", py.memoryForecast.predicted15mPct)
                            horizonPill("30m", py.memoryForecast.predicted30mPct ?? py.memoryForecast.predicted15mPct)
                        }

                        if let unc = py.memoryForecast.uncertaintyPct {
                            statRow("Uncertainty / MAE", String(format: "±%.1f%% (MAE %.2f%%)", unc, py.memoryForecast.validationMAE ?? 0))
                        }
                        statRow("Mem Slope", String(format: "%+.3f%%/min (%@)", py.memoryForecast.slopePctPerMin, py.memoryForecast.risk.uppercased()))

                        if let low = py.storageForecast.daysUntil90PctLow, let high = py.storageForecast.daysUntil90PctHigh {
                            statRow("Storage 90% Window", "\(low)–\(high) days (\(String(format: "%+.3f%%/d", py.storageForecast.growthPctPerDay)))")
                        } else {
                            statRow("Storage Growth", String(format: "%+.3f%%/day (%@)", py.storageForecast.growthPctPerDay, py.storageForecast.risk.uppercased()))
                        }
                        if let audit = client.storageAudit {
                            statRow("Storage Guard (50%)", String(format: "%.1f%% / 50%% · %.0f MB unused", audit.usedPct, audit.reclaimableMb))
                            if let items = audit.candidates, !items.isEmpty {
                                ForEach(items.prefix(3)) { item in
                                    statRow("  ↳ \(item.owner)", String(format: "%.0f MB · %dd old", item.sizeMb, item.ageDays))
                                }
                            }
                        }

                        if let reg = py.workloadRegime {
                            statRow("Workload Regime", "\(reg.regime) (\(reg.model))")
                        }
                        if let topAct = py.recommendedActions?.first {
                            statRow("Top Ranked Action", "\(topAct.action) (\(String(format: "%.0f%%", topAct.utility * 100)))")
                        }
                        if let trl = py.transformerRL {
                            statRow("Transformer Next", String(format: "CPU %.1f%% · Mem %.1f%% (Acc %.1f%%)", trl.predCPUPct, trl.predMemPct, trl.accuracyPct ?? max(0, 100 - trl.rmsePct)))
                            statRow("RL Policy Action", "\(trl.optimalAction) (Q=\(String(format: "%.2f", trl.qValue)), \(trl.episodes) ep)")
                        }
                        if let nce = client.cognitiveState {
                            Divider().overlay(HUDColors.cyanDim)
                            HStack(spacing: 6) {
                                statusPill("NCE: \(nce.schedulingTier)", color: nce.allowNeuralML ? HUDColors.green : HUDColors.amber)
                                statusPill("GRAPH: \(nce.graphEdgeCount) EDGES", color: HUDColors.cyan)
                                statusPill("MEM: \(nce.workingMemoryLen)W/\(nce.episodicMemoryLen)E", color: HUDColors.textMuted)
                            }
                            Text(nce.observation.summary)
                                .font(.system(size: 9, design: .monospaced))
                                .foregroundStyle(HUDColors.textMuted)
                                .lineLimit(2)
                        }

                        if let apps = py.abnormalApps, !apps.isEmpty {
                            Divider().overlay(HUDColors.cyanDim)
                            ForEach(apps) { app in
                                Button {
                                    client.sendPrompt("Inspect process \(app.name) (PID \(app.pid))")
                                    client.activeTab = AppScreen.assistant.rawValue
                                } label: {
                                    HStack {
                                        Text("⚠ \(app.name) (PID \(app.pid)): \(app.reason)")
                                            .font(.system(size: 9, design: .monospaced))
                                            .foregroundStyle(HUDColors.amber)
                                        Spacer()
                                        Text("→ Inspect")
                                            .font(.system(size: 8, weight: .bold, design: .monospaced))
                                            .foregroundStyle(HUDColors.cyan)
                                    }
                                }
                                .buttonStyle(.plain)
                            }
                        }
                    }
                } else {
                    Text("Waiting for Python scikit-learn forecast...")
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundStyle(HUDColors.textMuted)
                }
            }
        }
    }

    private func statusPill(_ text: String, color: Color) -> some View {
        Text(text)
            .font(.system(size: 8, weight: .bold, design: .monospaced))
            .foregroundStyle(color)
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(color.opacity(0.12))
            .overlay(RoundedRectangle(cornerRadius: 2).stroke(color.opacity(0.45), lineWidth: 1))
    }

    private func horizonPill(_ horizon: String, _ pct: Double) -> some View {
        let color = pct >= 85 ? HUDColors.red : (pct >= 70 ? HUDColors.amber : HUDColors.cyan)
        return VStack(spacing: 2) {
            Text(horizon)
                .font(.system(size: 8, design: .monospaced))
                .foregroundStyle(HUDColors.textMuted)
            Text(String(format: "%.1f%%", pct))
                .font(.system(size: 10, weight: .bold, design: .monospaced))
                .foregroundStyle(color)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 4)
        .background(color.opacity(0.08))
        .overlay(RoundedRectangle(cornerRadius: 3).stroke(color.opacity(0.35), lineWidth: 1))
    }

    private var processInspectorPanel: some View {
        let procs = filteredAndSortedProcesses
        let maxMem = max(1.0, Double(procs.map(\.memoryBytes).max() ?? 1))

        return HUDPanel(
            title: "PROCESS INSPECTOR",
            trailing: "\(procs.count) SHOWN · CLICK ROW TO INSPECT"
        ) {
            VStack(spacing: 8) {
                HStack(spacing: 10) {
                    HStack(spacing: 6) {
                        Image(systemName: "magnifyingglass")
                            .font(.system(size: 10))
                            .foregroundStyle(HUDColors.textMuted)
                        TextField("Filter processes by name or PID...", text: $client.processSearch)
                            .textFieldStyle(.plain)
                            .font(.system(size: 11, design: .monospaced))
                            .foregroundStyle(HUDColors.textPrimary)
                        if !client.processSearch.isEmpty {
                            Button {
                                client.processSearch = ""
                            } label: {
                                Image(systemName: "xmark.circle.fill")
                                    .font(.system(size: 10))
                                    .foregroundStyle(HUDColors.textMuted)
                            }
                            .buttonStyle(.plain)
                        }
                    }
                    .padding(.horizontal, 8)
                    .padding(.vertical, 5)
                    .background(HUDColors.background.opacity(0.6))
                    .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.cyanDim, lineWidth: 1))
                }

                if procs.isEmpty {
                    Text("No matching processes.")
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundStyle(HUDColors.textMuted)
                } else {
                    VStack(spacing: 5) {
                        HStack {
                            sortHeader("PROCESS", key: "NAME", alignment: .leading)
                                .frame(maxWidth: .infinity, alignment: .leading)
                            sortHeader("PID", key: "PID", alignment: .trailing)
                                .frame(width: 60, alignment: .trailing)
                            sortHeader("THREADS", key: "THREADS", alignment: .trailing)
                                .frame(width: 72, alignment: .trailing)
                            sortHeader("CPU", key: "CPU", alignment: .trailing)
                                .frame(width: 65, alignment: .trailing)
                            sortHeader("RSS", key: "RSS", alignment: .trailing)
                                .frame(width: 110, alignment: .trailing)
                        }

                        Divider().overlay(HUDColors.cyanDim)

                        ForEach(procs) { proc in
                            Button {
                                client.sendPrompt("Inspect process \(proc.name) (PID \(proc.pid))")
                                client.activeTab = AppScreen.assistant.rawValue
                            } label: {
                                HStack(spacing: 8) {
                                    Text(proc.name)
                                        .foregroundStyle(HUDColors.textPrimary)
                                        .frame(maxWidth: .infinity, alignment: .leading)
                                        .lineLimit(1)
                                    Text("\(proc.pid)")
                                        .foregroundStyle(HUDColors.textMuted)
                                        .frame(width: 60, alignment: .trailing)
                                    Text("\(proc.threads)")
                                        .foregroundStyle(HUDColors.textMuted)
                                        .frame(width: 72, alignment: .trailing)
                                    Text(String(format: "%.1f%%", proc.cpuPct))
                                        .foregroundStyle(proc.cpuPct > 40 ? HUDColors.amber : HUDColors.cyan)
                                        .frame(width: 65, alignment: .trailing)
                                    HStack(spacing: 5) {
                                        GeometryReader { geo in
                                            RoundedRectangle(cornerRadius: 2)
                                                .fill(HUDColors.cyan.opacity(0.65))
                                                .frame(width: max(2, geo.size.width * (Double(proc.memoryBytes) / maxMem)), height: 5)
                                                .frame(maxHeight: .infinity, alignment: .center)
                                        }
                                        .frame(width: 40, height: 12)
                                        Text(HUDFormatters.bytesMB(proc.memoryBytes))
                                            .foregroundStyle(HUDColors.textPrimary)
                                            .frame(width: 65, alignment: .trailing)
                                    }
                                    .frame(width: 110, alignment: .trailing)
                                }
                                .font(.system(size: 11, design: .monospaced))
                                .padding(.vertical, 3)
                                .contentShape(Rectangle())
                            }
                            .buttonStyle(.plain)
                        }
                    }
                }
            }
        }
    }

    private func sortHeader(_ title: String, key: String, alignment: Alignment) -> some View {
        let active = client.processSortKey == key
        let arrow = active ? (client.processSortAscending ? " ▲" : " ▼") : ""
        return Button {
            if client.processSortKey == key {
                client.processSortAscending.toggle()
            } else {
                client.processSortKey = key
                client.processSortAscending = false
            }
        } label: {
            Text(title + arrow)
                .font(.system(size: 9, weight: .bold, design: .monospaced))
                .foregroundStyle(active ? HUDColors.cyan : HUDColors.textMuted)
                .frame(maxWidth: .infinity, alignment: alignment)
        }
        .buttonStyle(.plain)
    }

    private func statRow(_ label: String, _ value: String) -> some View {
        HStack {
            Text(label)
                .font(.system(size: 10, design: .monospaced))
                .foregroundStyle(HUDColors.textMuted)
            Spacer()
            Text(value)
                .font(.system(size: 10, weight: .semibold, design: .monospaced))
                .foregroundStyle(HUDColors.textPrimary)
        }
    }
}

private struct SparklineGraph: View {
    let samples: [Double]
    let maxScale: Double
    let color: Color

    var body: some View {
        GeometryReader { geo in
            if samples.count >= 2 {
                let w = geo.size.width
                let h = geo.size.height
                let step = w / CGFloat(max(1, samples.count - 1))
                let denom = max(1.0, maxScale)

                ZStack {
                    Path { path in
                        for (i, val) in samples.enumerated() {
                            let x = CGFloat(i) * step
                            let clamped = min(1.0, max(0.0, val / denom))
                            let y = h - CGFloat(clamped) * (h - 4) - 2
                            if i == 0 {
                                path.move(to: CGPoint(x: x, y: h))
                                path.addLine(to: CGPoint(x: x, y: y))
                            } else {
                                path.addLine(to: CGPoint(x: x, y: y))
                            }
                        }
                        path.addLine(to: CGPoint(x: CGFloat(samples.count - 1) * step, y: h))
                        path.closeSubpath()
                    }
                    .fill(color.opacity(0.16))

                    Path { path in
                        for (i, val) in samples.enumerated() {
                            let x = CGFloat(i) * step
                            let clamped = min(1.0, max(0.0, val / denom))
                            let y = h - CGFloat(clamped) * (h - 4) - 2
                            if i == 0 {
                                path.move(to: CGPoint(x: x, y: y))
                            } else {
                                path.addLine(to: CGPoint(x: x, y: y))
                            }
                        }
                    }
                    .stroke(color, lineWidth: 1.5)
                }
            } else {
                Text("Collecting samples...")
                    .font(.system(size: 9, design: .monospaced))
                    .foregroundStyle(HUDColors.textMuted)
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
            }
        }
    }
}
