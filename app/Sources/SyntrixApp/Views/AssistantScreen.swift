import SwiftUI

struct AssistantScreen: View {
    @ObservedObject var client: SyntrixClient

    private let starterCards: [(title: String, subtitle: String, prompt: String, icon: String, accent: Color)] = [
        ("Causal Diagnosis", "Explain why CPU, RAM, or swap is elevated", "/diagnose", "waveform.path.ecg", HUDColors.cyan),
        ("Memory & Pressure", "Analyze RAM pressure & propose safe actions", "/optimize-memory", "memorychip", HUDColors.amber),
        ("Storage Forecast", "Predict disk growth & exhaustion window", "/storage-forecast", "chart.line.uptrend.xyaxis", HUDColors.green),
        ("Agent Action Plan", "Build a risk-gated step-by-step plan", "/plan Inspect system health and propose safe optimizations", "checklist", HUDColors.cyan)
    ]

    private var filteredActivity: [ActivityLine] {
        let recent = Array(client.activityLines.suffix(60))
        switch client.activityFilter {
        case "TOOLS":
            return recent.filter { $0.text.contains("[") || $0.text.localizedCaseInsensitiveContains("tool") || $0.text.localizedCaseInsensitiveContains("executed") }
        case "ALERTS":
            return recent.filter {
                $0.text.localizedCaseInsensitiveContains("error")
                    || $0.text.localizedCaseInsensitiveContains("warn")
                    || $0.text.localizedCaseInsensitiveContains("kill")
                    || $0.text.localizedCaseInsensitiveContains("pressure")
            }
        default:
            return recent
        }
    }

    var body: some View {
        VStack(spacing: 10) {
            liveContextStrip

            HStack(alignment: .top, spacing: 14) {
                // Main Conversation Area
                HUDPanel(title: "AI COMPANION & AGENTS", trailing: "\(client.transcript.count) MESSAGES") {
                    HStack(spacing: 10) {
                        Text("Offline Companion · Causal Diagnosis · Task Planner")
                            .font(.system(size: 10, design: .monospaced))
                            .foregroundStyle(HUDColors.textMuted)
                        Spacer()
                        Button {
                            client.sendPrompt("/help")
                        } label: {
                            Label("Commands", systemImage: "command")
                                .font(.system(size: 10, design: .monospaced))
                                .foregroundStyle(HUDColors.cyan)
                        }
                        .buttonStyle(.plain)

                        Button {
                            client.clearTranscript()
                        } label: {
                            Label("Clear", systemImage: "trash")
                                .font(.system(size: 10, design: .monospaced))
                                .foregroundStyle(HUDColors.textMuted)
                        }
                        .buttonStyle(.plain)
                    }

                    ScrollViewReader { proxy in
                        ScrollView {
                            VStack(alignment: .leading, spacing: 10) {
                                if client.transcript.count <= 1 {
                                    welcomeStarterGrid
                                }

                                ForEach(Array(client.transcript.enumerated()), id: \.element.id) { index, msg in
                                    messageCard(msg, isLatest: index == client.transcript.count - 1)
                                        .id(msg.id)
                                }

                                if client.isAssistantThinking {
                                    thinkingBubble
                                        .id("thinking-indicator")
                                }
                            }
                            .padding(.vertical, 4)
                        }
                        .onChange(of: client.transcript.count) { _, _ in
                            if let last = client.transcript.last {
                                withAnimation(.easeOut(duration: 0.2)) {
                                    proxy.scrollTo(last.id, anchor: .bottom)
                                }
                            }
                        }
                        .onChange(of: client.isAssistantThinking) { _, thinking in
                            if thinking {
                                withAnimation(.easeOut(duration: 0.2)) {
                                    proxy.scrollTo("thinking-indicator", anchor: .bottom)
                                }
                            }
                        }
                    }
                    .frame(maxHeight: .infinity)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)

                // Right Sidebar: Live Diagnosis + Daemon Activity Stream
                VStack(spacing: 10) {
                    liveDiagnosisCard
                    activityStreamPanel
                }
                .frame(minWidth: 290, idealWidth: 330, maxWidth: 370, maxHeight: .infinity)
            }
            .frame(maxHeight: .infinity)

            QuickCommandBar(client: client, switchToAssistantOnSend: false)
        }
    }

    private var liveContextStrip: some View {
        HStack(spacing: 10) {
            if let snap = client.snapshot {
                contextMetric(label: "CPU", value: String(format: "%.1f%%", snap.cpu.totalUsagePct),
                              color: snap.cpu.totalUsagePct > 80 ? HUDColors.red : HUDColors.cyan)
                contextMetric(label: "MEM PRESS", value: String(format: "%.0f%%", snap.memory.pressurePct),
                              color: snap.memory.pressurePct >= 75 ? HUDColors.red : (snap.memory.pressurePct >= 55 ? HUDColors.amber : HUDColors.green))
                contextMetric(label: "DISK", value: String(format: "%.0f%%", snap.disk.usedPct),
                              color: snap.disk.usedPct > 75 ? HUDColors.red : (snap.disk.usedPct > 50 ? HUDColors.amber : HUDColors.cyan))
                if let topProc = snap.topProcesses.first {
                    Button {
                        client.sendPrompt("inspect process \(topProc.name) (pid \(topProc.pid))")
                    } label: {
                        HStack(spacing: 4) {
                            Image(systemName: "flame.fill")
                                .font(.system(size: 9))
                            Text("TOP: \(topProc.name) (\(String(format: "%.0f MB", Double(topProc.memoryBytes) / 1_048_576.0)))")
                                .font(.system(size: 10, weight: .medium, design: .monospaced))
                        }
                        .padding(.horizontal, 8)
                        .padding(.vertical, 4)
                        .background(HUDColors.panelBg)
                        .foregroundStyle(HUDColors.amber)
                        .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.amber.opacity(0.45), lineWidth: 1))
                    }
                    .buttonStyle(.plain)
                    .help("Inspect top process in Assistant")
                }
            } else {
                Text("TELEMETRY: WARMING UP")
                    .font(.system(size: 10, design: .monospaced))
                    .foregroundStyle(HUDColors.textMuted)
            }

            Spacer()

            if !client.activeApp.isEmpty {
                Text("APP: \(client.activeApp)")
                    .font(.system(size: 10, design: .monospaced))
                    .foregroundStyle(HUDColors.textMuted)
            }

            let pendingRecs = client.recommendations.filter { $0.status == "pending" }
            if !pendingRecs.isEmpty {
                Button {
                    client.activeTab = AppScreen.overview.rawValue
                } label: {
                    HStack(spacing: 4) {
                        Image(systemName: "exclamationmark.triangle.fill")
                            .font(.system(size: 9))
                        Text("\(pendingRecs.count) PENDING ACTIONS")
                            .font(.system(size: 10, weight: .semibold, design: .monospaced))
                    }
                    .padding(.horizontal, 8)
                    .padding(.vertical, 4)
                    .background(HUDColors.amber.opacity(0.14))
                    .foregroundStyle(HUDColors.amber)
                    .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.amber, lineWidth: 1))
                }
                .buttonStyle(.plain)
            }

            Text("MODE: \(client.mode.rawValue)")
                .font(.system(size: 10, weight: .semibold, design: .monospaced))
                .foregroundStyle(HUDColors.cyan)
                .padding(.horizontal, 8)
                .padding(.vertical, 4)
                .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.cyanDim, lineWidth: 1))
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 7)
        .background(HUDColors.panelBg)
        .overlay(RoundedRectangle(cornerRadius: 4).stroke(HUDColors.cyanDim, lineWidth: 1))
    }

    private func contextMetric(label: String, value: String, color: Color) -> some View {
        HStack(spacing: 4) {
            Text(label)
                .font(.system(size: 9, design: .monospaced))
                .foregroundStyle(HUDColors.textMuted)
            Text(value)
                .font(.system(size: 10, weight: .bold, design: .monospaced))
                .foregroundStyle(color)
        }
        .padding(.horizontal, 7)
        .padding(.vertical, 3)
        .background(color.opacity(0.08))
        .overlay(RoundedRectangle(cornerRadius: 3).stroke(color.opacity(0.35), lineWidth: 1))
    }

    private var welcomeStarterGrid: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("QUICK START INTERACTIONS")
                .font(.system(size: 9, weight: .bold, design: .monospaced))
                .foregroundStyle(HUDColors.textMuted)

            LazyVGrid(columns: [GridItem(.flexible()), GridItem(.flexible())], spacing: 8) {
                ForEach(starterCards, id: \.title) { card in
                    Button {
                        client.sendPrompt(card.prompt)
                    } label: {
                        HStack(alignment: .top, spacing: 8) {
                            Image(systemName: card.icon)
                                .font(.system(size: 13))
                                .foregroundStyle(card.accent)
                                .frame(width: 20)
                            VStack(alignment: .leading, spacing: 2) {
                                Text(card.title)
                                    .font(.system(size: 11, weight: .semibold, design: .monospaced))
                                    .foregroundStyle(HUDColors.textPrimary)
                                Text(card.subtitle)
                                    .font(.system(size: 9, design: .monospaced))
                                    .foregroundStyle(HUDColors.textMuted)
                                    .lineLimit(2)
                            }
                            Spacer()
                        }
                        .padding(9)
                        .background(card.accent.opacity(0.07))
                        .overlay(RoundedRectangle(cornerRadius: 4).stroke(card.accent.opacity(0.35), lineWidth: 1))
                    }
                    .buttonStyle(.plain)
                }
            }
        }
        .padding(.bottom, 4)
    }

    private func messageCard(_ msg: TranscriptMessage, isLatest: Bool) -> some View {
        VStack(alignment: msg.isUser ? .trailing : .leading, spacing: 6) {
            HStack(spacing: 6) {
                if !msg.isUser {
                    Text("SYNTRIX · \(HUDFormatters.time.string(from: msg.timestamp))")
                        .font(.system(size: 9, weight: .semibold, design: .monospaced))
                        .foregroundStyle(HUDColors.cyan)
                    if let tool = msg.tool, !tool.isEmpty {
                        Text(tool.uppercased())
                            .font(.system(size: 8, weight: .bold, design: .monospaced))
                            .foregroundStyle(HUDColors.amber)
                            .padding(.horizontal, 5)
                            .padding(.vertical, 2)
                            .background(HUDColors.amber.opacity(0.12))
                            .overlay(RoundedRectangle(cornerRadius: 2).stroke(HUDColors.amber.opacity(0.4), lineWidth: 1))
                    }
                }
                Spacer()
                Button {
                    client.copyTranscriptMessage(msg)
                } label: {
                    HStack(spacing: 3) {
                        Image(systemName: client.copiedMessageID == msg.id ? "checkmark" : "doc.on.doc")
                            .font(.system(size: 8))
                        Text(client.copiedMessageID == msg.id ? "COPIED" : "COPY")
                            .font(.system(size: 8, weight: .semibold, design: .monospaced))
                    }
                    .foregroundStyle(client.copiedMessageID == msg.id ? HUDColors.green : HUDColors.textMuted)
                }
                .buttonStyle(.plain)

                if msg.isUser {
                    Text("YOU · \(HUDFormatters.time.string(from: msg.timestamp))")
                        .font(.system(size: 9, weight: .semibold, design: .monospaced))
                        .foregroundStyle(HUDColors.amber)
                }
            }

            Text(msg.text)
                .font(.system(size: 12, design: .monospaced))
                .foregroundStyle(HUDColors.textPrimary)
                .frame(maxWidth: .infinity, alignment: msg.isUser ? .trailing : .leading)
                .textSelection(.enabled)

            if !msg.isUser && isLatest && !msg.followUps.isEmpty {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 6) {
                        ForEach(msg.followUps) { fu in
                            Button {
                                client.sendPrompt(fu.prompt)
                            } label: {
                                HStack(spacing: 4) {
                                    Image(systemName: fu.icon)
                                        .font(.system(size: 9))
                                    Text(fu.label)
                                        .font(.system(size: 9, weight: .semibold, design: .monospaced))
                                }
                                .padding(.horizontal, 8)
                                .padding(.vertical, 4)
                                .background(HUDColors.cyan.opacity(0.1))
                                .foregroundStyle(HUDColors.cyan)
                                .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.cyan.opacity(0.45), lineWidth: 1))
                            }
                            .buttonStyle(.plain)
                        }
                    }
                }
                .padding(.top, 2)
            }
        }
        .padding(10)
        .background((msg.isUser ? HUDColors.amber : HUDColors.cyan).opacity(0.08))
        .overlay(
            RoundedRectangle(cornerRadius: 4)
                .stroke((msg.isUser ? HUDColors.amber : HUDColors.cyan).opacity(0.28), lineWidth: 1)
        )
    }

    private var thinkingBubble: some View {
        HStack(spacing: 8) {
            ProgressView()
                .scaleEffect(0.6)
                .frame(width: 12, height: 12)
            Text("SYNTRIX is analyzing live telemetry & running tools...")
                .font(.system(size: 11, design: .monospaced))
                .foregroundStyle(HUDColors.cyan)
            Spacer()
        }
        .padding(10)
        .background(HUDColors.cyan.opacity(0.06))
        .overlay(RoundedRectangle(cornerRadius: 4).stroke(HUDColors.cyanDim, lineWidth: 1))
    }

    private var liveDiagnosisCard: some View {
        HUDPanel(title: "CAUSAL INTELLIGENCE", trailing: client.diagnosis != nil ? String(format: "%.0f%% CONF", (client.diagnosis?.confidence ?? 0) * 100) : "IDLE") {
            if let diag = client.diagnosis {
                VStack(alignment: .leading, spacing: 6) {
                    Text(diag.rootCause)
                        .font(.system(size: 11, weight: .semibold, design: .monospaced))
                        .foregroundStyle(HUDColors.amber)
                    Text(diag.summary)
                        .font(.system(size: 10, design: .monospaced))
                        .foregroundStyle(HUDColors.textPrimary)
                        .lineLimit(3)
                    HStack(spacing: 8) {
                        Button("Explain Why") {
                            client.sendPrompt("/diagnose")
                        }
                        .font(.system(size: 9, weight: .semibold, design: .monospaced))
                        .foregroundStyle(HUDColors.cyan)
                        .padding(.horizontal, 7)
                        .padding(.vertical, 3)
                        .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.cyan, lineWidth: 1))
                        .buttonStyle(.plain)

                        Button("Build Plan") {
                            client.sendPrompt("/plan \(diag.rootCause)")
                        }
                        .font(.system(size: 9, weight: .semibold, design: .monospaced))
                        .foregroundStyle(HUDColors.amber)
                        .padding(.horizontal, 7)
                        .padding(.vertical, 3)
                        .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.amber, lineWidth: 1))
                        .buttonStyle(.plain)
                    }
                }
            } else {
                Text("Waiting for predictive diagnosis snapshot...")
                    .font(.system(size: 10, design: .monospaced))
                    .foregroundStyle(HUDColors.textMuted)
            }
        }
    }

    private var activityStreamPanel: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 6) {
                Circle().fill(Color.red.opacity(0.85)).frame(width: 8, height: 8)
                Circle().fill(Color.yellow.opacity(0.85)).frame(width: 8, height: 8)
                Circle().fill(Color.green.opacity(0.85)).frame(width: 8, height: 8)
                Spacer()
                Text("syntrix://activity")
                    .font(.system(size: 11, design: .monospaced))
                    .foregroundStyle(HUDColors.textMuted)
                Spacer()
                Text(client.isConnected ? "● live" : "○ idle")
                    .font(.system(size: 10, design: .monospaced))
                    .foregroundStyle(client.isConnected ? HUDColors.amber : HUDColors.textMuted)
            }

            HStack(spacing: 6) {
                ForEach(["ALL", "TOOLS", "ALERTS"], id: \.self) { f in
                    Button(f) {
                        client.activityFilter = f
                    }
                    .font(.system(size: 9, weight: .semibold, design: .monospaced))
                    .foregroundStyle(client.activityFilter == f ? HUDColors.cyan : HUDColors.textMuted)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(client.activityFilter == f ? HUDColors.cyan.opacity(0.14) : Color.clear)
                    .overlay(RoundedRectangle(cornerRadius: 3).stroke(client.activityFilter == f ? HUDColors.cyan : HUDColors.cyanDim, lineWidth: 1))
                    .buttonStyle(.plain)
                }
                Spacer()
                Button("Clear") {
                    client.clearActivity()
                }
                .font(.system(size: 9, design: .monospaced))
                .foregroundStyle(HUDColors.textMuted)
                .buttonStyle(.plain)
            }

            Divider().overlay(HUDColors.cyanDim)

            ScrollView {
                VStack(alignment: .leading, spacing: 5) {
                    if filteredActivity.isEmpty {
                        Text("No matching activity events.")
                            .font(.system(size: 10, design: .monospaced))
                            .foregroundStyle(HUDColors.textMuted)
                    } else {
                        ForEach(filteredActivity.reversed()) { line in
                            activityRow(line)
                        }
                    }
                }
            }
            .frame(maxHeight: .infinity)
        }
        .padding(12)
        .background(HUDColors.panelBg)
        .overlay(RoundedRectangle(cornerRadius: 4).stroke(HUDColors.cyanDim, lineWidth: 1))
        .overlay(HUDCorners())
        .frame(maxHeight: .infinity)
    }

    private func activityRow(_ line: ActivityLine) -> some View {
        let meta = formatActivity(line.text)
        let rowContent = VStack(alignment: .leading, spacing: 3) {
            HStack(spacing: 6) {
                Text(meta.badge)
                    .font(.system(size: 8, weight: .bold, design: .monospaced))
                    .foregroundStyle(meta.color)
                    .padding(.horizontal, 4)
                    .padding(.vertical, 1)
                    .background(meta.color.opacity(0.14))
                    .overlay(RoundedRectangle(cornerRadius: 2).stroke(meta.color.opacity(0.4), lineWidth: 1))

                Text(HUDFormatters.time.string(from: line.timestamp))
                    .font(.system(size: 9, design: .monospaced))
                    .foregroundStyle(HUDColors.textMuted)

                if line.count > 1 {
                    Text("×\(line.count)")
                        .font(.system(size: 8, weight: .bold, design: .monospaced))
                        .foregroundStyle(HUDColors.amber)
                        .padding(.horizontal, 4)
                        .padding(.vertical, 1)
                        .background(HUDColors.amber.opacity(0.14))
                        .cornerRadius(2)
                }

                Spacer()

                if let actionLabel = meta.actionLabel {
                    Text("→ \(actionLabel)")
                        .font(.system(size: 8, weight: .semibold, design: .monospaced))
                        .foregroundStyle(meta.color)
                }
            }

            Text(meta.summary)
                .font(.system(size: 10, design: .monospaced))
                .foregroundStyle(HUDColors.textPrimary)
                .frame(maxWidth: .infinity, alignment: .leading)
                .multilineTextAlignment(.leading)
        }
        .padding(6)
        .background(meta.color.opacity(0.05))
        .overlay(RoundedRectangle(cornerRadius: 3).stroke(meta.color.opacity(0.22), lineWidth: 1))

        return Group {
            if let prompt = meta.actionPrompt {
                Button {
                    client.sendPrompt(prompt)
                } label: {
                    rowContent
                }
                .buttonStyle(.plain)
            } else {
                rowContent
            }
        }
    }

    private func formatActivity(_ raw: String) -> (badge: String, summary: String, color: Color, actionPrompt: String?, actionLabel: String?) {
        if raw.hasPrefix("Detected: ") {
            let detail = String(raw.dropFirst("Detected: ".count))
            return ("ALERT", detail, HUDColors.amber, "/diagnose", "Diagnose")
        }
        if raw.hasPrefix("Dry-run preview: ") {
            return ("PREVIEW", String(raw.dropFirst("Dry-run preview: ".count)), HUDColors.cyan, nil, nil)
        }
        if raw.hasPrefix("Approved recommendation: ") {
            return ("APPROVED", String(raw.dropFirst("Approved recommendation: ".count)), HUDColors.green, "/audit", "Audit")
        }
        if raw.localizedCaseInsensitiveContains("kill switch") || raw.hasPrefix("Error:") {
            return ("SAFETY", raw, HUDColors.red, "/status", "Status")
        }
        if raw.hasPrefix("Mode changed to ") {
            return ("MODE", raw, HUDColors.cyan, nil, nil)
        }
        if raw.hasPrefix("[NSWorkspace]") {
            return ("CONTEXT", raw.replacingOccurrences(of: "[NSWorkspace] ", with: ""), HUDColors.textMuted, nil, nil)
        }
        if raw.hasPrefix("[tool:") || raw.contains("executed on prompt") {
            let cleaned = raw.replacingOccurrences(of: "[", with: "").replacingOccurrences(of: "] executed on prompt", with: " ran")
            return ("TOOL", cleaned, HUDColors.cyan, nil, nil)
        }
        if raw.hasPrefix("["), let closeIdx = raw.firstIndex(of: "]") {
            let body = String(raw[raw.index(after: closeIdx)...]).trimmingCharacters(in: .whitespaces)
            return ("EVENT", body.isEmpty ? raw : body, HUDColors.cyan, nil, nil)
        }
        return ("INFO", raw, HUDColors.textMuted, nil, nil)
    }
}
