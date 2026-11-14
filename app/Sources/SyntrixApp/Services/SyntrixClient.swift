import AppKit
import Combine
import Foundation

@MainActor
final class SyntrixClient: ObservableObject, @unchecked Sendable {
    @Published var isConnected: Bool = false
    @Published var mode: OperatingMode = .assist
    @Published var autoMode: Bool = false
    @Published var knowledgeDocs: Int = 0
    @Published var snapshot: SystemSnapshot?
    @Published var recommendations: [RecommendationItem] = []
    @Published var auditLogs: [AuditLogItem] = []
    @Published var storageForecast: StorageForecast?
    @Published var storageAudit: StorageAuditReport?
    @Published var workflowRuns: [WorkflowRunItem] = []
    @Published var activeApp: String = ""
    @Published var governorStatus: GovernorStatus?
    @Published var diagnosis: DiagnosisInfo?
    @Published var pythonForecast: PythonForecastInfo?
    @Published var selfHealth: SelfHealthInfo?
    @Published var guardianStatus: GuardianStatusInfo?
    @Published var cognitiveState: CognitiveStateInfo?
    @Published var contextCaptureEnabled: Bool = false {
        didSet { updateContextObserver() }
    }
    @Published var customWorkflowName: String = "Custom Mac Check"
    @Published var customSteps: [CustomWorkflowStep] = [
        CustomWorkflowStep(name: "System Status", tool: "system.status", condition: "always", requireApproval: false, maxRetries: 1),
        CustomWorkflowStep(name: "Inspect Caches", tool: "storage.inspect_caches", condition: "if_disk_gt:50", requireApproval: true, maxRetries: 1)
    ]
    @Published var inputText: String = ""
    @Published var isAssistantThinking: Bool = false
    @Published var activityFilter: String = "ALL"
    @Published var copiedMessageID: UUID? = nil
    @Published var cpuHistory: [Double] = []
    @Published var memPressureHistory: [Double] = []
    @Published var netInHistory: [Double] = []
    @Published var netOutHistory: [Double] = []
    @Published var isTelemetryPaused: Bool = false
    @Published var processSearch: String = ""
    @Published var processSortKey: String = "RSS"
    @Published var processSortAscending: Bool = false
    @Published var showAuditSheet: Bool = false
    @Published var showHealthTree: Bool = false
    @Published var selectedHealthMetric: String = "MEM"
    @Published var activeTab: String = "Overview"
    @Published var researchDir: String = "/Users/sahilkanungo/Desktop/SYNTRIX"
    @Published var researchQuery: String = ""
    @Published var researchOutput: String = "Index a local directory to search documentation and code offline."
    @Published var researchStats: ResearchPayload? = nil
    @Published var isResearchBusy: Bool = false
    @Published var researchTab: String = "HITS"
    @Published var workflowOutput: String = "Select a workflow template or build a custom pipeline to execute via the Event Bus and Tool Registry."
    @Published var isWorkflowBusy: Bool = false
    @Published var activeWorkflowLabel: String = ""
    @Published var transcript: [TranscriptMessage] = [
        TranscriptMessage(
            isUser: false,
            text: "Good evening. SYNTRIX systems are initializing. How may I help?",
            timestamp: Date()
        )
    ]
    @Published var activityLines: [ActivityLine] = [
        ActivityLine(timestamp: Date(), text: "SYNTRIX HUD initialized")
    ]

    private let socketPath: String
    private let readQueue = DispatchQueue(label: "com.syntrix.ipc.read", qos: .utility)
    private let writeQueue = DispatchQueue(label: "com.syntrix.ipc.write", qos: .userInitiated)
    nonisolated(unsafe) private var fd: Int32 = -1
    nonisolated(unsafe) private var isStopped = false
    private var workspaceObserver: NSObjectProtocol?

    init(socketPath: String = "/tmp/syntrixd.sock") {
        self.socketPath = socketPath
    }

    func start() {
        isStopped = false
        connectLoop()
    }

    func stop() {
        isStopped = true
        writeQueue.async { [weak self] in
            guard let self else { return }
            if self.fd >= 0 {
                Darwin.close(self.fd)
                self.fd = -1
            }
        }
    }

    func sendPrompt(_ prompt: String) {
        let trimmed = prompt.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        transcript.append(TranscriptMessage(isUser: true, text: trimmed, timestamp: Date()))
        isAssistantThinking = true
        sendRequest(["type": "chat", "prompt": trimmed])
    }

    func setMode(_ newMode: OperatingMode) {
        mode = newMode
        autoMode = false
        sendRequest(["type": "set_mode", "mode": newMode.rawValue])
    }

    func setAutoMode(_ enabled: Bool) {
        autoMode = enabled
        sendAnyRequest(["type": "set_auto_mode", "enabled": enabled])
    }

    func approveRecommendation(_ id: String) {
        recommendations.removeAll { $0.id == id }
        sendRequest(["type": "approve_rec", "rec_id": id])
    }

    func previewRecommendation(_ id: String) {
        sendRequest(["type": "preview_rec", "rec_id": id])
    }

    func activateKillSwitch() {
        sendRequest(["type": "kill_switch"])
    }

    func setGuardian(enabled: Bool, idleThresholdSec: Int) {
        sendAnyRequest(["type": "set_guardian", "enabled": enabled, "idle_threshold_sec": idleThresholdSec])
    }

    func dismissRecommendation(_ id: String) {
        recommendations.removeAll { $0.id == id }
        sendRequest(["type": "dismiss_rec", "rec_id": id])
    }

    func indexDirectory(_ dir: String) {
        isResearchBusy = true
        sendRequest(["type": "index_dir", "dir": dir])
    }

    func searchKnowledge(_ query: String) {
        let trimmed = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        researchQuery = trimmed
        isResearchBusy = true
        sendRequest(["type": "search_knowledge", "query": trimmed])
    }

    func runWorkflow(_ id: String, dir: String = "/Users/sahilkanungo/Desktop/SYNTRIX") {
        isWorkflowBusy = true
        activeWorkflowLabel = id
        sendRequest(["type": "run_workflow", "workflow_id": id, "dir": dir])
    }

    func runCustomWorkflow(approved: Bool = false) {
        isWorkflowBusy = true
        activeWorkflowLabel = customWorkflowName.isEmpty ? "Custom Workflow" : customWorkflowName
        let stepsPayload: [[String: Any]] = customSteps.map { step in
            [
                "name": step.name,
                "tool": step.tool,
                "condition": step.condition,
                "require_approval": step.requireApproval,
                "max_retries": step.maxRetries
            ]
        }
        let payload: [String: Any] = [
            "type": "run_custom_workflow",
            "approved": approved,
            "workflow": [
                "id": "custom-\(customWorkflowName.lowercased().replacingOccurrences(of: " ", with: "-"))",
                "name": customWorkflowName,
                "steps": stepsPayload
            ]
        ]
        sendAnyRequest(payload)
    }

    private func updateContextObserver() {
        if let obs = workspaceObserver {
            NSWorkspace.shared.notificationCenter.removeObserver(obs)
            workspaceObserver = nil
        }
        if !contextCaptureEnabled {
            activeApp = ""
            sendRequest(["type": "set_context", "active_app": "", "bundle_id": ""])
            return
        }
        if let front = NSWorkspace.shared.frontmostApplication {
            let name = front.localizedName ?? ""
            let bid = front.bundleIdentifier ?? ""
            activeApp = name
            sendRequest(["type": "set_context", "active_app": name, "bundle_id": bid])
        }
        workspaceObserver = NSWorkspace.shared.notificationCenter.addObserver(
            forName: NSWorkspace.didActivateApplicationNotification,
            object: nil,
            queue: .main
        ) { [weak self] note in
            guard let self else { return }
            let app = note.userInfo?[NSWorkspace.applicationUserInfoKey] as? NSRunningApplication
            let name = app?.localizedName ?? ""
            let bid = app?.bundleIdentifier ?? ""
            Task { @MainActor in
                self.activeApp = name
                self.sendRequest(["type": "set_context", "active_app": name, "bundle_id": bid])
            }
        }
    }

    func clearTranscript() {
        isAssistantThinking = false
        transcript = [
            TranscriptMessage(
                isUser: false,
                text: "Transcript cleared. Ask me anything about your Mac's performance, memory, or storage.",
                timestamp: Date(),
                tool: "system",
                followUps: [
                    AssistantFollowUp(label: "System Health", prompt: "/status", icon: "heart.text.square"),
                    AssistantFollowUp(label: "Causal Diagnosis", prompt: "/diagnose", icon: "waveform.path.ecg"),
                    AssistantFollowUp(label: "Top Processes", prompt: "/top", icon: "list.number")
                ]
            )
        ]
    }

    func pickResearchDirectory() {
        let panel = NSOpenPanel()
        panel.canChooseFiles = false
        panel.canChooseDirectories = true
        panel.allowsMultipleSelection = false
        panel.prompt = "Select Folder"
        if panel.runModal() == .OK, let url = panel.url {
            researchDir = url.path
        }
    }

    private func sendRequest(_ payload: [String: String]) {
        sendAnyRequest(payload)
    }

    private func sendAnyRequest(_ payload: [String: Any]) {
        guard let data = try? JSONSerialization.data(withJSONObject: payload) else { return }
        var line = data
        line.append(0x0A) // '\n'
        let payloadData = line

        writeQueue.async { [weak self] in
            guard let self, self.fd >= 0 else { return }
            payloadData.withUnsafeBytes { rawBuf in
                guard let base = rawBuf.baseAddress else { return }
                var totalSent = 0
                while totalSent < rawBuf.count {
                    let sent = Darwin.write(self.fd, base.advanced(by: totalSent), rawBuf.count - totalSent)
                    if sent <= 0 {
                        break
                    }
                    totalSent += sent
                }
            }
        }
    }

    private func connectLoop() {
        readQueue.async { [weak self] in
            guard let self else { return }
            while !self.isStopped {
                if self.openSocket() {
                    Task { @MainActor in
                        self.isConnected = true
                        self.appendActivity("Connected to syntrixd runtime")
                    }
                    self.readLoop()
                    Task { @MainActor in
                        self.isConnected = false
                        self.appendActivity("Disconnected from syntrixd; retrying...")
                    }
                }
                Thread.sleep(forTimeInterval: 2.0)
            }
        }
    }

    nonisolated private func openSocket() -> Bool {
        let sock = Darwin.socket(AF_UNIX, SOCK_STREAM, 0)
        guard sock >= 0 else { return false }

        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        let pathBytes = Array(socketPath.utf8CString)
        let maxLen = MemoryLayout.size(ofValue: addr.sun_path)
        guard pathBytes.count <= maxLen else {
            Darwin.close(sock)
            return false
        }
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
        if rc != 0 {
            Darwin.close(sock)
            return false
        }

        self.fd = sock
        return true
    }

    nonisolated private func readLoop() {
        var buffer = Data()
        var chunk = [UInt8](repeating: 0, count: 8192)

        while !isStopped && fd >= 0 {
            let n = Darwin.read(fd, &chunk, chunk.count)
            if n <= 0 {
                break
            }
            buffer.append(contentsOf: chunk[0..<n])
            while let newlineIdx = buffer.firstIndex(of: 0x0A) {
                let lineData = buffer.subdata(in: 0..<newlineIdx)
                buffer.removeSubrange(0...newlineIdx)
                if !lineData.isEmpty {
                    handleLineData(lineData)
                }
            }
        }

        if fd >= 0 {
            Darwin.close(fd)
            fd = -1
        }
    }

    nonisolated private func handleLineData(_ data: Data) {
        guard let msg = try? JSONDecoder().decode(ServerMessage.self, from: data) else {
            return
        }
        Task { @MainActor in
            switch msg.type {
            case "telemetry":
                if let t = msg.telemetry {
                    if self.mode != t.mode {
                        self.mode = t.mode
                    }
                    if let am = t.autoMode, self.autoMode != am {
                        self.autoMode = am
                    }
                    if let kd = t.knowledgeDocs, self.knowledgeDocs != kd {
                        self.knowledgeDocs = kd
                    }
                    if !self.isTelemetryPaused {
                        self.snapshot = t.snapshot
                        self.appendHistory(&self.cpuHistory, value: t.snapshot.cpu.totalUsagePct)
                        self.appendHistory(&self.memPressureHistory, value: t.snapshot.memory.pressurePct)
                        self.appendHistory(&self.netInHistory, value: t.snapshot.network.bytesInPerSec / 1024.0)
                        self.appendHistory(&self.netOutHistory, value: t.snapshot.network.bytesOutPerSec / 1024.0)
                    }
                    let newRecs = t.recommendations ?? []
                    if self.recommendations != newRecs {
                        self.recommendations = newRecs
                    }
                    let newLogs = t.auditLogs ?? []
                    if self.auditLogs != newLogs {
                        self.auditLogs = newLogs
                    }
                    if self.storageForecast != t.storageForecast {
                        self.storageForecast = t.storageForecast
                    }
                    self.storageAudit = t.storageAudit
                    let newRuns = t.workflowRuns ?? []
                    if self.workflowRuns != newRuns {
                        self.workflowRuns = newRuns
                    }
                    self.governorStatus = t.governor
                    self.diagnosis = t.diagnosis
                    self.pythonForecast = t.pythonForecast
                    self.selfHealth = t.selfHealth
                    self.guardianStatus = t.guardian
                    self.cognitiveState = t.cognitive
                    if let app = t.activeApp, self.activeApp != app {
                        self.activeApp = app
                    }
                }
            case "chat_reply":
                self.isAssistantThinking = false
                if let text = msg.text {
                    let followUps = Self.suggestedFollowUps(forTool: msg.tool)
                    self.transcript.append(
                        TranscriptMessage(
                            isUser: false,
                            text: text,
                            timestamp: Date(),
                            tool: msg.tool,
                            followUps: followUps
                        )
                    )
                    if self.transcript.count > 50 {
                        self.transcript.removeFirst(self.transcript.count - 50)
                    }
                }
            case "mode_changed":
                if let raw = msg.text, let newMode = OperatingMode(rawValue: raw) {
                    self.mode = newMode
                }
            case "activity":
                if let text = msg.text {
                    self.appendActivity(text)
                }
            case "notification":
                if let notif = msg.notification {
                    self.appendActivity("macOS Notification: \(notif.title)")
                    NotificationService.shared.postInactiveAppNotification(notif)
                }
            case "research_result":
                self.isResearchBusy = false
                if let text = msg.text {
                    self.researchOutput = text
                }
                if let r = msg.research {
                    self.researchStats = r
                }
            case "workflow_result":
                self.isWorkflowBusy = false
                if let text = msg.text {
                    self.workflowOutput = text
                }
            case "error":
                self.isAssistantThinking = false
                self.isResearchBusy = false
                self.isWorkflowBusy = false
                if let err = msg.error {
                    self.appendActivity("Error: \(err)")
                }
            default:
                break
            }
        }
    }

    func clearActivity() {
        activityLines.removeAll()
    }

    func copyTranscriptMessage(_ msg: TranscriptMessage) {
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(msg.text, forType: .string)
        copiedMessageID = msg.id
        DispatchQueue.main.asyncAfter(deadline: .now() + 1.5) { [weak self] in
            if self?.copiedMessageID == msg.id {
                self?.copiedMessageID = nil
            }
        }
    }

    private func appendHistory(_ arr: inout [Double], value: Double) {
        arr.append(value)
        if arr.count > 60 {
            arr.removeFirst(arr.count - 60)
        }
    }

    private func appendActivity(_ text: String) {
        if let last = activityLines.last, last.text == text {
            activityLines[activityLines.count - 1] = ActivityLine(
                timestamp: Date(),
                text: text,
                count: last.count + 1
            )
            return
        }
        activityLines.append(ActivityLine(timestamp: Date(), text: text))
        if activityLines.count > 100 {
            activityLines.removeFirst(activityLines.count - 100)
        }
    }

    private static func suggestedFollowUps(forTool tool: String?) -> [AssistantFollowUp] {
        switch tool {
        case "tool:get_system_status":
            return [
                AssistantFollowUp(label: "Causal Diagnosis", prompt: "/diagnose", icon: "waveform.path.ecg"),
                AssistantFollowUp(label: "Top Processes", prompt: "/top", icon: "list.number"),
                AssistantFollowUp(label: "Forecast Storage", prompt: "/storage-forecast", icon: "chart.line.uptrend.xyaxis")
            ]
        case "tool:diagnosis":
            return [
                AssistantFollowUp(label: "Optimization Scan", prompt: "/scan", icon: "slider.horizontal.3"),
                AssistantFollowUp(label: "Top Processes", prompt: "/top", icon: "list.number"),
                AssistantFollowUp(label: "Build Action Plan", prompt: "/plan Resolve current system pressure safely", icon: "checklist")
            ]
        case "event:memory_optimization", "tool:run_optimization_scan":
            return [
                AssistantFollowUp(label: "Causal Diagnosis", prompt: "/diagnose", icon: "waveform.path.ecg"),
                AssistantFollowUp(label: "Check Caches", prompt: "/caches", icon: "trash.slash"),
                AssistantFollowUp(label: "Audit Log", prompt: "/audit", icon: "shield.checkered")
            ]
        case "tool:list_top_processes", "tool:inspect_process":
            return [
                AssistantFollowUp(label: "Diagnose Pressure", prompt: "/diagnose", icon: "waveform.path.ecg"),
                AssistantFollowUp(label: "Optimize Memory", prompt: "/optimize-memory", icon: "memorychip")
            ]
        case "tool:inspect_caches", "tool:storage.forecast":
            return [
                AssistantFollowUp(label: "Scan System", prompt: "/scan", icon: "slider.horizontal.3"),
                AssistantFollowUp(label: "Startup Agents", prompt: "/startup", icon: "power")
            ]
        case "tool:plan":
            return [
                AssistantFollowUp(label: "Run System Scan", prompt: "/scan", icon: "slider.horizontal.3"),
                AssistantFollowUp(label: "Check Audit", prompt: "/audit", icon: "shield.checkered")
            ]
        default:
            return [
                AssistantFollowUp(label: "System Health", prompt: "/status", icon: "heart.text.square"),
                AssistantFollowUp(label: "Causal Diagnosis", prompt: "/diagnose", icon: "waveform.path.ecg")
            ]
        }
    }
}
