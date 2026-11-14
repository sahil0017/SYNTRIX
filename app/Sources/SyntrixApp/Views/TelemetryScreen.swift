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
        VStack(spacing: 10) {
            telemetryControlBar

            if let snap = client.snapshot {
                sparklineRow(snap)
            }

            telemetrySubTabBar

            Group {
                switch client.telemetrySubTab {
                case 1:
                    mlAndAdvisorTab
                case 2:
                    processesAndGovernorTab
                default:
                    systemAndCoresTab
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        }
    }

    private var telemetrySubTabBar: some View {
        HStack(spacing: 8) {
            subTabButton(index: 0, title: "SYSTEM & CORES", icon: "cpu")
            subTabButton(index: 1, title: "ML & ADVISOR", icon: "brain.head.profile")
            subTabButton(index: 2, title: "PROCESSES & GOVERNOR", icon: "list.bullet.rectangle.portrait")
            Spacer()
        }
    }

    private func subTabButton(index: Int, title: String, icon: String) -> some View {
        let active = client.telemetrySubTab == index
        return Button {
            client.telemetrySubTab = index
        } label: {
            HStack(spacing: 5) {
                Image(systemName: icon)
                    .font(.system(size: 10))
                Text(title)
                    .font(.system(size: 10, weight: .bold, design: .monospaced))
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 6)
            .background(active ? HUDColors.cyan.opacity(0.18) : HUDColors.panelBg)
            .foregroundStyle(active ? HUDColors.cyan : HUDColors.textMuted)
            .overlay(
                RoundedRectangle(cornerRadius: 4)
                    .stroke(active ? HUDColors.cyan : HUDColors.cyanDim, lineWidth: 1)
            )
            .cornerRadius(4)
        }
        .buttonStyle(.plain)
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
                .padding(.vertical, 4)
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
        .padding(.vertical, 6)
        .background(HUDColors.panelBg)
        .overlay(RoundedRectangle(cornerRadius: 4).stroke(HUDColors.cyanDim, lineWidth: 1))
    }

    private func sparklineRow(_ snap: SystemSnapshot) -> some View {
        HStack(spacing: 10) {
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

            let healthScore = client.pythonForecast?.superIntelligence?.healthIndex.healthScore ?? (client.mlHealthHistory.last ?? max(10.0, 100.0 - snap.memory.pressurePct * 0.5 - snap.cpu.totalUsagePct * 0.3))
            let bottleneck = client.pythonForecast?.superIntelligence?.bottleneck.dominantState ?? "Nominal"
            let healthColor = healthScore >= 75 ? HUDColors.green : (healthScore >= 50 ? HUDColors.amber : HUDColors.red)
            sparklineCard(
                title: "ML HEALTH & GPU",
                value: String(format: "%.0f/100", healthScore),
                subtitle: String(format: "%@ · GPU %.0f%%", bottleneck, snap.gpu?.deviceUtilPct ?? 0),
                samples: client.mlHealthHistory,
                maxScale: 100.0,
                color: healthColor
            )
        }
    }

    private func sparklineCard(title: String, value: String, subtitle: String, samples: [Double], maxScale: Double, color: Color) -> some View {
        VStack(alignment: .leading, spacing: 4) {
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
                .frame(height: 28)

            Text(subtitle)
                .font(.system(size: 9, design: .monospaced))
                .foregroundStyle(HUDColors.textMuted)
                .lineLimit(1)
        }
        .padding(8)
        .background(HUDColors.panelBg)
        .overlay(RoundedRectangle(cornerRadius: 4).stroke(color.opacity(0.35), lineWidth: 1))
        .overlay(HUDCorners())
        .frame(maxWidth: .infinity)
    }

    // MARK: - Sub-Tab 0: System & Cores
    private var systemAndCoresTab: some View {
        HStack(alignment: .top, spacing: 12) {
            HUDPanel(title: "HARDWARE & MEMORY", trailing: client.snapshot.map { "\($0.cpu.coreCount) CORES" }) {
                if let snap = client.snapshot {
                    VStack(alignment: .leading, spacing: 9) {
                        HUDProgressBar(label: "CPU", valuePct: snap.cpu.totalUsagePct, accent: snap.cpu.totalUsagePct > 75 ? HUDColors.red : HUDColors.amber)
                        HUDProgressBar(label: "MEM", valuePct: snap.memory.pressurePct, accent: snap.memory.pressurePct > 75 ? HUDColors.red : HUDColors.cyan)
                        HUDProgressBar(label: "DSK", valuePct: snap.disk.usedPct, accent: snap.disk.usedPct > 75 ? HUDColors.red : (snap.disk.usedPct > 50 ? HUDColors.amber : HUDColors.cyan))

                        HStack(spacing: 10) {
                            let rMB = (snap.disk.readBytesPerSec ?? 0) / (1024 * 1024)
                            let wMB = (snap.disk.writeBytesPerSec ?? 0) / (1024 * 1024)
                            let iops = (snap.disk.readIops ?? 0) + (snap.disk.writeIops ?? 0)
                            Text(String(format: "SSD R %.1f MB/s · W %.1f MB/s · %.0f IOPS", rMB, wMB, iops))
                                .font(.system(size: 9.5, design: .monospaced))
                                .foregroundStyle((rMB + wMB >= 250 || iops >= 5000) ? HUDColors.amber : HUDColors.textMuted)
                            Spacer()
                            if let gpu = snap.gpu, gpu.available {
                                Text(String(format: "GPU %.0f%% (Ren %.0f%%)", gpu.deviceUtilPct, gpu.rendererUtilPct))
                                    .font(.system(size: 9.5, weight: .semibold, design: .monospaced))
                                    .foregroundStyle(gpu.deviceUtilPct >= 80 ? HUDColors.amber : HUDColors.cyan)
                            }
                        }

                        Divider().overlay(HUDColors.cyanDim)
                        memoryCompositionBar(snap.memory)

                        let pgIn = snap.memory.pageinsPerSec ?? 0
                        let pgOut = snap.memory.pageoutsPerSec ?? 0
                        statRow("Swap Thrash (pgIn / pgOut)", String(format: "%.0f/s · %.0f/s", pgIn, pgOut))
                        statRow("Memory Used / Total", "\(HUDFormatters.bytesGB(snap.memory.usedBytes)) / \(HUDFormatters.bytesGB(snap.memory.totalBytes))")
                        statRow("Compressed / Swap", "\(HUDFormatters.bytesMB(snap.memory.compressedBytes)) / \(HUDFormatters.bytesMB(snap.memory.swapUsedBytes))")
                        statRow("Thermal / Power", "\((snap.thermal?.stateLabel ?? "nominal").uppercased()) (\(Int(snap.thermal?.cpuSpeedLimitPct ?? 100))%) · " + (snap.power.hasBattery ? "\(Int(snap.power.batteryPct))%" : "AC"))
                    }
                } else {
                    Text("Waiting for C17 telemetry snapshot...")
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundStyle(HUDColors.textMuted)
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)

            VStack(spacing: 12) {
                HUDPanel(title: "PER-CORE CPU UTILIZATION", trailing: client.snapshot.map { "\($0.cpu.perCorePct.count) CORES" }) {
                    if let snap = client.snapshot, !snap.cpu.perCorePct.isEmpty {
                        LazyVGrid(columns: [GridItem(.flexible()), GridItem(.flexible())], spacing: 8) {
                            ForEach(Array(snap.cpu.perCorePct.enumerated()), id: \.offset) { idx, pct in
                                let coreColor = pct > 80 ? HUDColors.red : (pct > 50 ? HUDColors.amber : HUDColors.cyan)
                                HStack(spacing: 6) {
                                    Text("CORE \(idx)")
                                        .font(.system(size: 9.5, weight: .bold, design: .monospaced))
                                        .foregroundStyle(HUDColors.textMuted)
                                        .frame(width: 46, alignment: .leading)
                                    GeometryReader { geo in
                                        ZStack(alignment: .leading) {
                                            RoundedRectangle(cornerRadius: 2).fill(HUDColors.cyanDim.opacity(0.3))
                                            RoundedRectangle(cornerRadius: 2)
                                                .fill(coreColor)
                                                .frame(width: max(2, geo.size.width * min(1.0, pct / 100.0)))
                                        }
                                    }
                                    .frame(height: 7)
                                    Text(String(format: "%3.0f%%", pct))
                                        .font(.system(size: 9.5, weight: .semibold, design: .monospaced))
                                        .foregroundStyle(coreColor)
                                        .frame(width: 34, alignment: .trailing)
                                }
                            }
                        }
                    } else {
                        Text("Collecting per-core telemetry...")
                            .font(.system(size: 10, design: .monospaced))
                            .foregroundStyle(HUDColors.textMuted)
                    }
                }

                HUDPanel(title: "STORAGE GUARD (50% LIMIT)", trailing: client.storageAudit.map { String(format: "%.1f%% USED", $0.usedPct) }) {
                    if let audit = client.storageAudit {
                        VStack(alignment: .leading, spacing: 6) {
                            statRow("Reclaimable Unused", String(format: "%.0f MB · %.1f GB free", audit.reclaimableMb, audit.freeGb))
                            if let items = audit.candidates, !items.isEmpty {
                                ForEach(items.prefix(3)) { item in
                                    statRow("↳ \(item.owner)", String(format: "%.0f MB · %dd old", item.sizeMb, item.ageDays))
                                }
                            }
                            if let cleanMsg = client.lastStorageCleanSummary, !cleanMsg.isEmpty {
                                Text(cleanMsg)
                                    .font(.system(size: 9, design: .monospaced))
                                    .foregroundStyle(HUDColors.green)
                                    .lineLimit(2)
                            }
                            HStack(spacing: 8) {
                                Button {
                                    client.cleanStorage()
                                } label: {
                                    HStack(spacing: 4) {
                                        if client.isCleaningStorage {
                                            ProgressView()
                                                .scaleEffect(0.5)
                                                .frame(width: 10, height: 10)
                                        } else {
                                            Image(systemName: "trash.slash.fill")
                                                .font(.system(size: 9))
                                        }
                                        Text(client.isCleaningStorage ? "CLEANING..." : "CLEAN STALE FILES")
                                            .font(.system(size: 9, weight: .bold, design: .monospaced))
                                    }
                                    .foregroundStyle(HUDColors.green)
                                    .padding(.horizontal, 8)
                                    .padding(.vertical, 4)
                                    .background(HUDColors.green.opacity(0.14))
                                    .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.green.opacity(0.5), lineWidth: 1))
                                }
                                .buttonStyle(.plain)
                                .disabled(client.isCleaningStorage)

                                Button {
                                    client.sendPrompt("/caches")
                                    client.activeTab = AppScreen.assistant.rawValue
                                } label: {
                                    Text("INSPECT CACHES")
                                        .font(.system(size: 9, weight: .bold, design: .monospaced))
                                        .foregroundStyle(HUDColors.cyan)
                                        .padding(.horizontal, 8)
                                        .padding(.vertical, 4)
                                        .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.cyan.opacity(0.5), lineWidth: 1))
                                }
                                .buttonStyle(.plain)
                                Spacer()
                            }
                            .padding(.top, 2)
                        }
                    } else {
                        Text("Storage audit ready on next scan.")
                            .font(.system(size: 10, design: .monospaced))
                            .foregroundStyle(HUDColors.textMuted)
                    }
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        }
    }

    // MARK: - Sub-Tab 1: ML & Advisor
    private var mlAndAdvisorTab: some View {
        HStack(alignment: .top, spacing: 12) {
            HUDPanel(title: "PYTHON ML & 7/7 NEURAL SUBSYSTEMS", trailing: client.pythonForecast.map { "\($0.sampleCount) SAMPLES" }) {
                if let py = client.pythonForecast {
                    VStack(alignment: .leading, spacing: 8) {
                        HStack(spacing: 6) {
                            statusPill(py.memoryForecast.selectedModel ?? py.modelBackend, color: HUDColors.cyan)
                            if let drift = py.memoryForecast.driftDetected {
                                statusPill(drift ? "DRIFT DETECTED" : "MODEL STABLE", color: drift ? HUDColors.amber : HUDColors.green)
                            }
                            if let si = py.superIntelligence {
                                let sc = si.healthIndex.healthScore
                                let c = sc >= 75 ? HUDColors.green : (sc >= 50 ? HUDColors.amber : HUDColors.red)
                                statusPill(String(format: "SI HEALTH %.0f/100", sc), color: c)
                                statusPill(si.bottleneck.dominantState.uppercased(), color: si.bottleneck.dominantState == "Nominal" ? HUDColors.green : HUDColors.amber)
                            }
                        }

                        HStack(spacing: 6) {
                            horizonPill("1m", py.memoryForecast.predicted1mPct ?? py.memoryForecast.currentPct)
                            horizonPill("5m", py.memoryForecast.predicted5mPct)
                            horizonPill("15m", py.memoryForecast.predicted15mPct)
                            horizonPill("30m", py.memoryForecast.predicted30mPct ?? py.memoryForecast.predicted15mPct)
                        }

                        if let trl = py.transformerRL {
                            statRow("2-Layer Transformer + Double-Q RL", String(format: "CPU %.0f%% · Mem %.0f%% · %@ (Q=%.2f)", trl.predCPUPct, trl.predMemPct, trl.optimalAction, trl.qValue))
                        }

                        if let ns = py.neuralSubsystems {
                            Divider().overlay(HUDColors.cyanDim)
                            LazyVGrid(columns: [GridItem(.flexible()), GridItem(.flexible())], spacing: 6) {
                                miniNeuralCell("1. RESOURCE (TCN/GRU)", String(format: "%.0f%% / %.0f%% (%@)", ns.resourceIntelligence.tcnPredMemPct, ns.resourceIntelligence.gruPredMemPct, ns.resourceIntelligence.selectedModel), HUDColors.cyan)
                                miniNeuralCell("2. BEHAVIOUR (LSTM-AE)", String(format: "MSE %.3f · %d anom (%@)", ns.behaviourIntelligence.maxReconError, ns.behaviourIntelligence.anomalousCount, ns.behaviourIntelligence.topProcess), ns.behaviourIntelligence.anomalousCount > 0 ? HUDColors.amber : HUDColors.green)
                                miniNeuralCell("3. CONTEXT PATTERN", "\(ns.contextIntelligence.workingPattern) (\(Int(ns.contextIntelligence.confidence * 100))%)", HUDColors.green)
                                miniNeuralCell("4. CAUSAL DAG", ns.causalIntelligence.rootCause, HUDColors.amber)
                                miniNeuralCell("5. INTENT SLM", ns.intentModel, HUDColors.cyan)
                                miniNeuralCell("6. DECISION & 7. RAG", "\(ns.decisionIntelligence.topAction) · \(ns.knowledgeModel)", HUDColors.cyan)
                            }
                        }
                    }
                } else {
                    mlProgressView(title: "Running Python ML & 7/7 Neural Pipeline")
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)

            HUDPanel(title: "APP ADVISOR (UNDERSTAND & IMPROVE)", trailing: "PYTHON ML") {
                if let adv = client.pythonForecast?.appAdvisor {
                    VStack(alignment: .leading, spacing: 8) {
                        Text(adv.plainEnglishSummary)
                            .font(.system(size: 10.5, weight: .semibold, design: .monospaced))
                            .foregroundStyle(HUDColors.textPrimary)

                        if !adv.understandingNotes.isEmpty {
                            Divider().overlay(HUDColors.cyanDim)
                            ForEach(adv.understandingNotes, id: \.self) { note in
                                Text("• \(note)")
                                    .font(.system(size: 9.5, design: .monospaced))
                                    .foregroundStyle(HUDColors.textMuted)
                            }
                        }

                        if !adv.improvementTips.isEmpty {
                            Divider().overlay(HUDColors.cyanDim)
                            ForEach(adv.improvementTips) { tip in
                                Button {
                                    client.sendPrompt(tip.command)
                                    client.activeTab = AppScreen.assistant.rawValue
                                } label: {
                                    HStack(spacing: 8) {
                                        Image(systemName: "sparkles")
                                            .font(.system(size: 10))
                                            .foregroundStyle(HUDColors.cyan)
                                        VStack(alignment: .leading, spacing: 2) {
                                            Text(tip.title)
                                                .font(.system(size: 9.5, weight: .bold, design: .monospaced))
                                                .foregroundStyle(HUDColors.cyan)
                                            Text(tip.action)
                                                .font(.system(size: 9, design: .monospaced))
                                                .foregroundStyle(HUDColors.textMuted)
                                        }
                                        Spacer()
                                        Text(tip.command)
                                            .font(.system(size: 8.5, weight: .bold, design: .monospaced))
                                            .foregroundStyle(HUDColors.green)
                                    }
                                    .padding(7)
                                    .background(HUDColors.cyan.opacity(0.08))
                                    .cornerRadius(4)
                                }
                                .buttonStyle(.plain)
                            }
                        }
                    }
                } else {
                    mlProgressView(title: "Preparing App Advisor Insights")
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        }
    }

    private func mlProgressView(title: String) -> some View {
        let prog = client.mlProgress
        let pct = max(5.0, min(100.0, prog?.progressPct ?? 10.0))
        let elapsedSec = Double(prog?.elapsedMs ?? 0) / 1000.0
        let estSec = max(0.5, Double(prog?.estimatedMs ?? 3200) / 1000.0)
        let stage = prog?.stage ?? "Initializing sklearn + Transformer..."

        return VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text(title)
                    .font(.system(size: 10.5, weight: .semibold, design: .monospaced))
                    .foregroundStyle(HUDColors.cyan)
                Spacer()
                Text(String(format: "%.1fs / ~%.1fs (%.0f%%)", elapsedSec, estSec, pct))
                    .font(.system(size: 10, weight: .bold, design: .monospaced))
                    .foregroundStyle(HUDColors.amber)
            }

            HUDProgressBar(label: "ML", valuePct: pct, accent: HUDColors.cyan)

            Text(stage)
                .font(.system(size: 9.5, design: .monospaced))
                .foregroundStyle(HUDColors.textMuted)

            if let err = prog?.lastError, !err.isEmpty {
                Text("Last ML error: \(err)")
                    .font(.system(size: 9, design: .monospaced))
                    .foregroundStyle(HUDColors.red)
                    .lineLimit(2)
            }
        }
        .padding(.vertical, 6)
    }

    // MARK: - Sub-Tab 2: Processes & Governor
    private var processesAndGovernorTab: some View {
        HStack(alignment: .top, spacing: 12) {
            VStack(spacing: 12) {
                HUDPanel(title: "RESOURCE GOVERNOR", trailing: client.governorStatus?.state ?? "INIT") {
                    if let gov = client.governorStatus {
                        VStack(alignment: .leading, spacing: 6) {
                            HStack(spacing: 6) {
                                statusPill("STATE: \(gov.state)", color: gov.state == "NORMAL" ? HUDColors.green : HUDColors.amber)
                                statusPill(gov.allowIndexing ? "INDEX: ON" : "INDEX: PAUSED", color: gov.allowIndexing ? HUDColors.green : HUDColors.amber)
                                statusPill(gov.allowInference ? "ML: ON" : "ML: OFF", color: gov.allowInference ? HUDColors.green : HUDColors.red)
                            }
                            statRow("Host Mem Pressure", String(format: "%.1f%%", gov.memoryPressurePct))
                            statRow("Daemon Heap / Goroutines", String(format: "%.2f MB / %d", gov.goHeapAllocMB, gov.goroutines))
                            statRow("Max Concurrent Agents", "\(gov.maxConcurrentAgents)")
                        }
                    } else {
                        Text("Governor pending...")
                            .font(.system(size: 10, design: .monospaced))
                            .foregroundStyle(HUDColors.textMuted)
                    }
                }

                HUDPanel(title: "DAEMON SELF-HEALTH & NCE", trailing: "OBSERVABILITY") {
                    VStack(alignment: .leading, spacing: 6) {
                        if let sh = client.selfHealth {
                            statRow("C17 Collector Latency", "\(sh.collectorLatencyUs) µs")
                            statRow("SQLite Write Latency", "\(sh.sqliteWriteUs) µs")
                            statRow("Journaled Events", "\(sh.eventCount)")
                        }
                        if let nce = client.cognitiveState {
                            Divider().overlay(HUDColors.cyanDim)
                            statRow("NCE Tier / Graph", "\(nce.schedulingTier) · \(nce.graphEdgeCount) edges")
                            statRow("Working / Episodic Mem", "\(nce.workingMemoryLen)W / \(nce.episodicMemoryLen)E")
                        }
                    }
                }
            }
            .frame(width: 340)
            .frame(maxHeight: .infinity, alignment: .top)

            processInspectorPanel
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    private func memoryCompositionBar(_ mem: MemoryMetrics) -> some View {
        let total = max(1.0, Double(mem.totalBytes))
        let activeFrac = Double(mem.activeBytes) / total
        let wiredFrac = Double(mem.wiredBytes) / total
        let compFrac = Double(mem.compressedBytes) / total

        return VStack(alignment: .leading, spacing: 4) {
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
        HStack(spacing: 3) {
            Circle().fill(color).frame(width: 5, height: 5)
            Text(label)
                .font(.system(size: 8, design: .monospaced))
                .foregroundStyle(HUDColors.textMuted)
        }
    }

    private func miniNeuralCell(_ label: String, _ value: String, _ color: Color) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(label)
                .font(.system(size: 8, weight: .bold, design: .monospaced))
                .foregroundStyle(HUDColors.textMuted)
            Text(value)
                .font(.system(size: 9.5, weight: .semibold, design: .monospaced))
                .foregroundStyle(color)
                .lineLimit(1)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, 7)
        .padding(.vertical, 5)
        .background(color.opacity(0.08))
        .cornerRadius(3)
    }

    private func statusPill(_ text: String, color: Color) -> some View {
        Text(text)
            .font(.system(size: 8, weight: .bold, design: .monospaced))
            .foregroundStyle(color)
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(color.opacity(0.12))
            .overlay(RoundedRectangle(cornerRadius: 2).stroke(color.opacity(0.45), lineWidth: 1))
            .lineLimit(1)
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
                HStack(spacing: 8) {
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

                if procs.isEmpty {
                    Text("No matching processes.")
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundStyle(HUDColors.textMuted)
                    Spacer()
                } else {
                    HStack {
                        sortHeader("PROCESS", key: "NAME", alignment: .leading)
                            .frame(maxWidth: .infinity, alignment: .leading)
                        sortHeader("PID", key: "PID", alignment: .trailing)
                            .frame(width: 50, alignment: .trailing)
                        sortHeader("CPU", key: "CPU", alignment: .trailing)
                            .frame(width: 55, alignment: .trailing)
                        Text("I/O · WKUP")
                            .font(.system(size: 8.5, weight: .bold, design: .monospaced))
                            .foregroundStyle(HUDColors.textMuted)
                            .frame(width: 90, alignment: .trailing)
                        sortHeader("FOOTPRINT", key: "RSS", alignment: .trailing)
                            .frame(width: 100, alignment: .trailing)
                    }

                    Divider().overlay(HUDColors.cyanDim)

                    ScrollView {
                        VStack(spacing: 4) {
                            ForEach(procs) { proc in
                                let footprint = proc.physFootprintBytes ?? proc.memoryBytes
                                let ioBps = (proc.diskReadBps ?? 0) + (proc.diskWriteBps ?? 0)
                                let wk = proc.wakeupsPerSec ?? 0
                                let pg = proc.pageinsPerSec ?? 0
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
                                            .frame(width: 50, alignment: .trailing)
                                        Text(String(format: "%.1f%%", proc.cpuPct))
                                            .foregroundStyle(proc.cpuPct > 40 ? HUDColors.amber : HUDColors.cyan)
                                            .frame(width: 55, alignment: .trailing)
                                        Text(pg >= 10 ? String(format: "%.0f pg/s", pg) : (ioBps >= 1024*1024 ? String(format: "%.1fM/s", ioBps/(1024*1024)) : String(format: "%.0f wk/s", wk)))
                                            .foregroundStyle(pg >= 40 ? HUDColors.red : (wk >= 250 ? HUDColors.amber : HUDColors.textMuted))
                                            .frame(width: 90, alignment: .trailing)
                                        HStack(spacing: 5) {
                                            GeometryReader { geo in
                                                RoundedRectangle(cornerRadius: 2)
                                                    .fill(HUDColors.cyan.opacity(0.65))
                                                    .frame(width: max(2, geo.size.width * (Double(footprint) / maxMem)), height: 5)
                                                    .frame(maxHeight: .infinity, alignment: .center)
                                            }
                                            .frame(width: 34, height: 12)
                                            Text(HUDFormatters.bytesMB(footprint))
                                                .foregroundStyle(HUDColors.textPrimary)
                                                .frame(width: 60, alignment: .trailing)
                                        }
                                        .frame(width: 100, alignment: .trailing)
                                    }
                                    .font(.system(size: 10.5, design: .monospaced))
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
                .font(.system(size: 8.5, weight: .bold, design: .monospaced))
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
                .lineLimit(1)
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
