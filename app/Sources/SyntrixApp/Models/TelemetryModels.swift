import Foundation

enum OperatingMode: String, CaseIterable, Codable, Identifiable {
    case off = "OFF"
    case monitor = "MONITOR"
    case assist = "ASSIST"
    case autopilot = "AUTOPILOT"
    case learning = "LEARNING"

    var id: String { rawValue }
}

struct CPUMetrics: Codable {
    let totalUsagePct: Double
    let userPct: Double
    let systemPct: Double
    let idlePct: Double
    let coreCount: Int
    let perCorePct: [Double]

    enum CodingKeys: String, CodingKey {
        case totalUsagePct = "total_usage_pct"
        case userPct = "user_pct"
        case systemPct = "system_pct"
        case idlePct = "idle_pct"
        case coreCount = "core_count"
        case perCorePct = "per_core_pct"
    }
}

struct MemoryMetrics: Codable {
    let totalBytes: UInt64
    let usedBytes: UInt64
    let activeBytes: UInt64
    let inactiveBytes: UInt64
    let wiredBytes: UInt64
    let compressedBytes: UInt64
    let freeBytes: UInt64
    let swapTotalBytes: UInt64
    let swapUsedBytes: UInt64
    let usedPct: Double
    let pressurePct: Double
    let pressureLevel: String
    let pageinsPerSec: Double?
    let pageoutsPerSec: Double?

    enum CodingKeys: String, CodingKey {
        case totalBytes = "total_bytes"
        case usedBytes = "used_bytes"
        case activeBytes = "active_bytes"
        case inactiveBytes = "inactive_bytes"
        case wiredBytes = "wired_bytes"
        case compressedBytes = "compressed_bytes"
        case freeBytes = "free_bytes"
        case swapTotalBytes = "swap_total_bytes"
        case swapUsedBytes = "swap_used_bytes"
        case usedPct = "used_pct"
        case pressurePct = "pressure_pct"
        case pressureLevel = "pressure_level"
        case pageinsPerSec = "pageins_per_sec"
        case pageoutsPerSec = "pageouts_per_sec"
    }
}

struct DiskMetrics: Codable {
    let totalBytes: UInt64
    let freeBytes: UInt64
    let availableBytes: UInt64
    let usedPct: Double
    let readBytesPerSec: Double?
    let writeBytesPerSec: Double?
    let readIops: Double?
    let writeIops: Double?

    enum CodingKeys: String, CodingKey {
        case totalBytes = "total_bytes"
        case freeBytes = "free_bytes"
        case availableBytes = "available_bytes"
        case usedPct = "used_pct"
        case readBytesPerSec = "read_bytes_per_sec"
        case writeBytesPerSec = "write_bytes_per_sec"
        case readIops = "read_iops"
        case writeIops = "write_iops"
    }
}

struct NetworkMetrics: Codable {
    let totalBytesIn: UInt64
    let totalBytesOut: UInt64
    let bytesInPerSec: Double
    let bytesOutPerSec: Double

    enum CodingKeys: String, CodingKey {
        case totalBytesIn = "total_bytes_in"
        case totalBytesOut = "total_bytes_out"
        case bytesInPerSec = "bytes_in_per_sec"
        case bytesOutPerSec = "bytes_out_per_sec"
    }
}

struct PowerMetrics: Codable {
    let hasBattery: Bool
    let batteryPct: Double
    let isCharging: Bool
    let onACPower: Bool

    enum CodingKeys: String, CodingKey {
        case hasBattery = "has_battery"
        case batteryPct = "battery_pct"
        case isCharging = "is_charging"
        case onACPower = "on_ac_power"
    }
}

struct ThermalMetrics: Codable {
    let thermalLevel: Int32
    let cpuSpeedLimitPct: Double
    let isThrottled: Bool
    let stateLabel: String

    enum CodingKeys: String, CodingKey {
        case thermalLevel = "thermal_level"
        case cpuSpeedLimitPct = "cpu_speed_limit_pct"
        case isThrottled = "is_throttled"
        case stateLabel = "state_label"
    }
}

struct ProcessInfoItem: Codable, Identifiable {
    let pid: Int32
    let ppid: Int32
    let name: String
    let cpuPct: Double
    let memoryBytes: UInt64
    let threads: UInt32
    let physFootprintBytes: UInt64?
    let diskReadBps: Double?
    let diskWriteBps: Double?
    let pageinsPerSec: Double?
    let wakeupsPerSec: Double?

    var id: Int32 { pid }

    enum CodingKeys: String, CodingKey {
        case pid, ppid, name, threads
        case cpuPct = "cpu_pct"
        case memoryBytes = "memory_bytes"
        case physFootprintBytes = "phys_footprint_bytes"
        case diskReadBps = "disk_read_bps"
        case diskWriteBps = "disk_write_bps"
        case pageinsPerSec = "pageins_per_sec"
        case wakeupsPerSec = "wakeups_per_sec"
    }
}

struct GPUMetrics: Codable {
    let available: Bool
    let deviceUtilPct: Double
    let rendererUtilPct: Double
    let tilerUtilPct: Double
    let inUseMemoryBytes: UInt64

    enum CodingKeys: String, CodingKey {
        case available
        case deviceUtilPct = "device_util_pct"
        case rendererUtilPct = "renderer_util_pct"
        case tilerUtilPct = "tiler_util_pct"
        case inUseMemoryBytes = "in_use_memory_bytes"
    }
}

struct SystemSnapshot: Codable {
    let timestampMs: Int64
    let cpu: CPUMetrics
    let memory: MemoryMetrics
    let disk: DiskMetrics
    let network: NetworkMetrics
    let power: PowerMetrics
    let thermal: ThermalMetrics?
    let gpu: GPUMetrics?
    let processCountTotal: UInt32
    let topProcesses: [ProcessInfoItem]

    enum CodingKeys: String, CodingKey {
        case timestampMs = "timestamp_ms"
        case cpu, memory, disk, network, power, thermal, gpu
        case processCountTotal = "process_count_total"
        case topProcesses = "top_processes"
    }
}

struct RecommendationItem: Codable, Identifiable, Equatable {
    let id: String
    let createdAtMs: Int64
    let kind: String
    let severity: String
    let title: String
    let detail: String
    let targetPid: Int32
    let targetName: String
    let status: String
    let beforeMetric: String
    let afterMetric: String

    enum CodingKeys: String, CodingKey {
        case id, kind, severity, title, detail, status
        case createdAtMs = "created_at_ms"
        case targetPid = "target_pid"
        case targetName = "target_name"
        case beforeMetric = "before_metric"
        case afterMetric = "after_metric"
    }
}

struct AuditLogItem: Codable, Identifiable, Equatable {
    let id: Int64
    let timestampMs: Int64
    let actor: String
    let action: String
    let target: String
    let status: String
    let detail: String

    enum CodingKeys: String, CodingKey {
        case id, actor, action, target, status, detail
        case timestampMs = "timestamp_ms"
    }
}

struct StorageForecast: Codable, Equatable {
    let currentUsedPct: Double
    let growthPctPerDay: Double
    let daysUntil90Pct: Int
    let summary: String

    enum CodingKeys: String, CodingKey {
        case currentUsedPct = "current_used_pct"
        case growthPctPerDay = "growth_pct_per_day"
        case daysUntil90Pct = "days_until_90_pct"
        case summary
    }
}

struct WorkflowRunItem: Codable, Identifiable, Equatable {
    let id: String
    let name: String
    let status: String
    let currentStep: Int
    let totalSteps: Int
    let output: String
    let updatedAtMs: Int64

    enum CodingKeys: String, CodingKey {
        case id, name, status, output
        case currentStep = "current_step"
        case totalSteps = "total_steps"
        case updatedAtMs = "updated_at_ms"
    }
}

struct CustomWorkflowStep: Identifiable, Codable {
    var id = UUID()
    var name: String
    var tool: String
    var condition: String
    var requireApproval: Bool
    var maxRetries: Int

    enum CodingKeys: String, CodingKey {
        case name, tool, condition
        case requireApproval = "require_approval"
        case maxRetries = "max_retries"
    }
}

struct GovernorStatus: Codable {
    let state: String
    let memoryPressurePct: Double
    let goHeapAllocMB: Double
    let goroutines: Int
    let maxConcurrentAgents: Int
    let allowIndexing: Bool
    let allowInference: Bool

    enum CodingKeys: String, CodingKey {
        case state, goroutines
        case memoryPressurePct = "memory_pressure_pct"
        case goHeapAllocMB = "go_heap_alloc_mb"
        case maxConcurrentAgents = "max_concurrent_agents"
        case allowIndexing = "allow_indexing"
        case allowInference = "allow_inference"
    }
}

struct SuggestedActionInfo: Codable, Identifiable {
    var id: String { label + prompt }
    let label: String
    let prompt: String
    let kind: String
}

struct DiagnosisInfo: Codable {
    let summary: String
    let rootCause: String
    let impact: String?
    let confidence: Double
    let evidence: [String]?
    let healthTree: [String]?
    let recommendedActions: [SuggestedActionInfo]?

    enum CodingKeys: String, CodingKey {
        case summary, impact, confidence, evidence
        case rootCause = "root_cause"
        case healthTree = "health_tree"
        case recommendedActions = "recommended_actions"
    }
}

struct SelfHealthInfo: Codable {
    let collectorLatencyUs: UInt64
    let sqliteWriteUs: UInt64
    let eventCount: UInt64
    let heapAllocMB: Double
    let goroutines: Int

    enum CodingKeys: String, CodingKey {
        case goroutines
        case collectorLatencyUs = "collector_latency_us"
        case sqliteWriteUs = "sqlite_write_us"
        case eventCount = "event_count"
        case heapAllocMB = "heap_alloc_mb"
    }
}

struct PythonMemoryForecast: Codable {
    let currentPct: Double
    let predicted1mPct: Double?
    let predicted5mPct: Double
    let predicted15mPct: Double
    let predicted30mPct: Double?
    let uncertaintyPct: Double?
    let slopePctPerMin: Double
    let minutesToCritical: Double
    let selectedModel: String?
    let validationMAE: Double?
    let driftDetected: Bool?
    let driftScore: Double?
    let risk: String

    enum CodingKeys: String, CodingKey {
        case risk
        case currentPct = "current_pct"
        case predicted1mPct = "predicted_1m_pct"
        case predicted5mPct = "predicted_5m_pct"
        case predicted15mPct = "predicted_15m_pct"
        case predicted30mPct = "predicted_30m_pct"
        case uncertaintyPct = "uncertainty_pct"
        case slopePctPerMin = "slope_pct_per_min"
        case minutesToCritical = "minutes_to_critical"
        case selectedModel = "selected_model"
        case validationMAE = "validation_mae"
        case driftDetected = "drift_detected"
        case driftScore = "drift_score"
    }
}

struct PythonStorageForecast: Codable {
    let currentPct: Double
    let growthPctPerDay: Double
    let daysUntil90Pct: Int
    let daysUntil90PctLow: Int?
    let daysUntil90PctHigh: Int?
    let risk: String

    enum CodingKeys: String, CodingKey {
        case risk
        case currentPct = "current_pct"
        case growthPctPerDay = "growth_pct_per_day"
        case daysUntil90Pct = "days_until_90_pct"
        case daysUntil90PctLow = "days_until_90_pct_low"
        case daysUntil90PctHigh = "days_until_90_pct_high"
    }
}

struct AbnormalAppInfo: Codable, Identifiable {
    var id: String { "\(pid)-\(name)" }
    let name: String
    let pid: Int32
    let cpuPct: Double
    let memMB: Double
    let anomalyScore: Double
    let reason: String

    enum CodingKeys: String, CodingKey {
        case name, pid, reason
        case cpuPct = "cpu_pct"
        case memMB = "mem_mb"
        case anomalyScore = "anomaly_score"
    }
}

struct WorkloadRegimeInfo: Codable {
    let regime: String
    let clusterId: Int
    let model: String

    enum CodingKeys: String, CodingKey {
        case regime, model
        case clusterId = "cluster_id"
    }
}

struct RankedActionInfo: Codable, Identifiable {
    var id: String { action }
    let action: String
    let utility: Double
}

struct TransformerRLInfo: Codable {
    let predCPUPct: Double
    let predMemPct: Double
    let rmsePct: Double
    let accuracyPct: Double?
    let optimalAction: String
    let qValue: Double
    let episodes: Int
    let avgReward: Double
    let sqliteRowsUsed: Int

    enum CodingKeys: String, CodingKey {
        case predCPUPct = "transformer_pred_cpu_pct"
        case predMemPct = "transformer_pred_mem_pct"
        case rmsePct = "transformer_rmse_pct"
        case accuracyPct = "transformer_accuracy_pct"
        case optimalAction = "rl_optimal_action"
        case qValue = "rl_q_value"
        case episodes = "rl_episodes"
        case avgReward = "rl_avg_reward"
        case sqliteRowsUsed = "sqlite_rows_used"
    }
}

struct BottleneckClassificationInfo: Codable {
    let dominantState: String
    let confidence: Double
    let culprit: String
    let model: String

    enum CodingKeys: String, CodingKey {
        case culprit, model, confidence
        case dominantState = "dominant_state"
    }
}

struct PCAHealthIndexInfo: Codable {
    let healthScore: Double
    let primaryVarianceDriver: String
    let pcaExplainedVariance: Double
    let model: String

    enum CodingKeys: String, CodingKey {
        case model
        case healthScore = "health_score"
        case primaryVarianceDriver = "primary_variance_driver"
        case pcaExplainedVariance = "pca_explained_variance"
    }
}

struct StrategicDirectiveInfo: Codable {
    let recommendedDirective: String
    let expectedHealthGainPct: Double
    let rationale: String
    let policy: String

    enum CodingKeys: String, CodingKey {
        case rationale, policy
        case recommendedDirective = "recommended_directive"
        case expectedHealthGainPct = "expected_health_gain_pct"
    }
}

struct ProcessHangRiskInfo: Codable, Identifiable {
    let name: String
    let pid: Int32
    let hangRiskScore: Double
    let cause: String
    var id: Int32 { pid }

    enum CodingKeys: String, CodingKey {
        case name, pid, cause
        case hangRiskScore = "hang_risk_score"
    }
}

struct SuperIntelligenceInfo: Codable {
    let bottleneck: BottleneckClassificationInfo
    let healthIndex: PCAHealthIndexInfo
    let strategicDirective: StrategicDirectiveInfo
    let hangRiskScore: Double?
    let atRiskProcesses: [ProcessHangRiskInfo]?
    let storageUrgencyScore: Double?
    let inferenceMs: Double?

    enum CodingKeys: String, CodingKey {
        case bottleneck
        case healthIndex = "health_index"
        case strategicDirective = "strategic_directive"
        case hangRiskScore = "hang_risk_score"
        case atRiskProcesses = "at_risk_processes"
        case storageUrgencyScore = "storage_urgency_score"
        case inferenceMs = "inference_ms"
    }
}

struct ResourceNeuralInfo: Codable {
    let tcnPredMemPct: Double
    let gruPredMemPct: Double
    let selectedModel: String
    let ensemblePredMemPct: Double
    let architecture: String

    enum CodingKeys: String, CodingKey {
        case architecture
        case tcnPredMemPct = "tcn_pred_mem_pct"
        case gruPredMemPct = "gru_pred_mem_pct"
        case selectedModel = "selected_model"
        case ensemblePredMemPct = "ensemble_pred_mem_pct"
    }
}

struct BehaviourNeuralInfo: Codable {
    let maxReconError: Double
    let anomalousCount: Int
    let topProcess: String
    let model: String

    enum CodingKeys: String, CodingKey {
        case model
        case maxReconError = "max_recon_error"
        case anomalousCount = "anomalous_count"
        case topProcess = "top_process"
    }
}

struct ContextNeuralInfo: Codable {
    let workingPattern: String
    let confidence: Double
    let sequenceStability: Double
    let model: String

    enum CodingKeys: String, CodingKey {
        case confidence, model
        case workingPattern = "working_pattern"
        case sequenceStability = "sequence_stability"
    }
}

struct CausalNeuralInfo: Codable {
    let rootCause: String
    let causalChain: [String]
    let contributingFactors: [String]
    let causalStrength: Double
    let model: String

    enum CodingKeys: String, CodingKey {
        case model
        case rootCause = "root_cause"
        case causalChain = "causal_chain"
        case contributingFactors = "contributing_factors"
        case causalStrength = "causal_strength"
    }
}

struct DecisionNeuralInfo: Codable {
    let topAction: String
    let rootCauseContext: String
    let policy: String

    enum CodingKeys: String, CodingKey {
        case policy
        case topAction = "top_action"
        case rootCauseContext = "root_cause_context"
    }
}

struct NeuralSubsystemsInfo: Codable {
    let resourceIntelligence: ResourceNeuralInfo
    let behaviourIntelligence: BehaviourNeuralInfo
    let contextIntelligence: ContextNeuralInfo
    let causalIntelligence: CausalNeuralInfo
    let decisionIntelligence: DecisionNeuralInfo
    let intentModel: String
    let knowledgeModel: String
    let latencyMs: Double

    enum CodingKeys: String, CodingKey {
        case resourceIntelligence = "resource_intelligence"
        case behaviourIntelligence = "behaviour_intelligence"
        case contextIntelligence = "context_intelligence"
        case causalIntelligence = "causal_intelligence"
        case decisionIntelligence = "decision_intelligence"
        case intentModel = "intent_model"
        case knowledgeModel = "knowledge_model"
        case latencyMs = "latency_ms"
    }
}

struct ImprovementTipInfo: Codable, Identifiable {
    let category: String
    let title: String
    let action: String
    let command: String
    var id: String { "\(category)-\(title)" }
}

struct AppAdvisorInfo: Codable {
    let plainEnglishSummary: String
    let understandingNotes: [String]
    let improvementTips: [ImprovementTipInfo]

    enum CodingKeys: String, CodingKey {
        case plainEnglishSummary = "plain_english_summary"
        case understandingNotes = "understanding_notes"
        case improvementTips = "improvement_tips"
    }
}

struct PythonForecastInfo: Codable {
    let modelBackend: String
    let modelVersion: String?
    let featureSchemaVersion: String?
    let sampleCount: Int
    let memoryForecast: PythonMemoryForecast
    let storageForecast: PythonStorageForecast
    let workloadRegime: WorkloadRegimeInfo?
    let recommendedActions: [RankedActionInfo]?
    let transformerRL: TransformerRLInfo?
    let superIntelligence: SuperIntelligenceInfo?
    let neuralSubsystems: NeuralSubsystemsInfo?
    let appAdvisor: AppAdvisorInfo?
    let abnormalApps: [AbnormalAppInfo]?
    let summary: String

    enum CodingKeys: String, CodingKey {
        case summary
        case modelBackend = "model_backend"
        case modelVersion = "model_version"
        case featureSchemaVersion = "feature_schema_version"
        case sampleCount = "sample_count"
        case memoryForecast = "memory_forecast"
        case storageForecast = "storage_forecast"
        case workloadRegime = "workload_regime"
        case recommendedActions = "recommended_actions"
        case transformerRL = "transformer_rl"
        case superIntelligence = "super_intelligence"
        case neuralSubsystems = "neural_subsystems"
        case appAdvisor = "app_advisor"
        case abnormalApps = "abnormal_apps"
    }
}

struct GuardianStatusInfo: Codable {
    let enabled: Bool
    let state: String
    let idleSeconds: Int
    let idleThresholdSec: Int
    let actionsTaken: Int
    let proactivePrevented: Int?
    let skippedUnsafe: Int
    let rlAvgReward: Double?
    let devBoostActive: Bool?
    let focusContext: String?
    let lastAction: String
    let lastCheckMs: Int64

    enum CodingKeys: String, CodingKey {
        case enabled, state
        case idleSeconds = "idle_seconds"
        case idleThresholdSec = "idle_threshold_sec"
        case actionsTaken = "actions_taken"
        case proactivePrevented = "proactive_prevented"
        case skippedUnsafe = "skipped_unsafe"
        case rlAvgReward = "rl_avg_reward"
        case devBoostActive = "dev_boost_active"
        case focusContext = "focus_context"
        case lastAction = "last_action"
        case lastCheckMs = "last_check_ms"
    }
}

struct PerceptionObservationInfo: Codable {
    let contentionScore: Double
    let primaryCandidate: String
    let summary: String
    let confidence: Double

    enum CodingKeys: String, CodingKey {
        case summary, confidence
        case contentionScore = "contention_score"
        case primaryCandidate = "primary_candidate"
    }
}

struct CognitiveStateInfo: Codable {
    let schedulingTier: String
    let allowNeuralML: Bool
    let observation: PerceptionObservationInfo
    let graphEdgeCount: Int
    let workingMemoryLen: Int
    let episodicMemoryLen: Int

    enum CodingKeys: String, CodingKey {
        case observation
        case schedulingTier = "scheduling_tier"
        case allowNeuralML = "allow_neural_ml"
        case graphEdgeCount = "graph_edge_count"
        case workingMemoryLen = "working_memory_len"
        case episodicMemoryLen = "episodic_memory_len"
    }
}

struct StorageCleanupCandidate: Codable, Identifiable {
    var id: String { path }
    let path: String
    let category: String
    let owner: String
    let sizeMb: Double
    let fileCount: Int
    let ageDays: Int
    let safeToAuto: Bool
    let reason: String

    enum CodingKeys: String, CodingKey {
        case path, category, owner, reason
        case sizeMb = "size_mb"
        case fileCount = "file_count"
        case ageDays = "age_days"
        case safeToAuto = "safe_to_auto"
    }
}

struct StorageAuditReport: Codable {
    let usedPct: Double
    let thresholdPct: Double
    let exceedsLimit: Bool
    let freeGb: Double
    let totalGb: Double
    let reclaimableMb: Double
    let summary: String
    let scannedAtMs: Int64
    let candidates: [StorageCleanupCandidate]?

    enum CodingKeys: String, CodingKey {
        case summary, candidates
        case usedPct = "used_pct"
        case thresholdPct = "threshold_pct"
        case exceedsLimit = "exceeds_limit"
        case freeGb = "free_gb"
        case totalGb = "total_gb"
        case reclaimableMb = "reclaimable_mb"
        case scannedAtMs = "scanned_at_ms"
    }
}

struct MLProgressInfo: Codable {
    let evaluating: Bool
    let elapsedMs: Int64
    let estimatedMs: Int64
    let progressPct: Double
    let stage: String
    let lastError: String?

    enum CodingKeys: String, CodingKey {
        case evaluating, stage
        case elapsedMs = "elapsed_ms"
        case estimatedMs = "estimated_ms"
        case progressPct = "progress_pct"
        case lastError = "last_error"
    }
}

struct TelemetryPayload: Codable {
    let mode: OperatingMode
    let autoMode: Bool?
    let knowledgeDocs: Int?
    let snapshot: SystemSnapshot
    let recommendations: [RecommendationItem]?
    let auditLogs: [AuditLogItem]?
    let storageForecast: StorageForecast?
    let storageAudit: StorageAuditReport?
    let activeApp: String?
    let workflowRuns: [WorkflowRunItem]?
    let governor: GovernorStatus?
    let diagnosis: DiagnosisInfo?
    let pythonForecast: PythonForecastInfo?
    let selfHealth: SelfHealthInfo?
    let guardian: GuardianStatusInfo?
    let cognitive: CognitiveStateInfo?
    let mlProgress: MLProgressInfo?
    let mlHealthHistory: [Double]?

    enum CodingKeys: String, CodingKey {
        case mode, snapshot, recommendations, governor, diagnosis, guardian, cognitive
        case autoMode = "auto_mode"
        case knowledgeDocs = "knowledge_docs"
        case auditLogs = "audit_logs"
        case storageForecast = "storage_forecast"
        case storageAudit = "storage_audit"
        case activeApp = "active_app"
        case workflowRuns = "workflow_runs"
        case pythonForecast = "python_forecast"
        case selfHealth = "self_health"
        case mlProgress = "ml_progress"
        case mlHealthHistory = "ml_health_history"
    }
}

struct ResearchHitItem: Codable, Identifiable {
    var id: String { "\(docPath):\(score):\(snippet.prefix(24))" }
    let docPath: String
    let title: String
    let score: Double
    let snippet: String

    enum CodingKeys: String, CodingKey {
        case title, score, snippet
        case docPath = "doc_path"
    }
}

struct KnowledgeGraphEdgeItem: Codable, Identifiable {
    var id: String { "\(from)->\(to):\(kind)" }
    let from: String
    let to: String
    let kind: String
}

struct ResearchPayload: Codable {
    let docCount: Int
    let chunkCount: Int
    let edgeCount: Int
    let topConcepts: [String]?
    let hits: [ResearchHitItem]?
    let graphEdges: [KnowledgeGraphEdgeItem]?

    enum CodingKeys: String, CodingKey {
        case hits
        case docCount = "doc_count"
        case chunkCount = "chunk_count"
        case edgeCount = "edge_count"
        case topConcepts = "top_concepts"
        case graphEdges = "graph_edges"
    }
}

struct InactiveAppNotificationInfo: Codable {
    let id: String
    let title: String
    let subtitle: String?
    let body: String
    let recId: String
    let targetPid: Int32
    let targetName: String
    let rssMb: Double
    let activeApp: String

    enum CodingKeys: String, CodingKey {
        case id, title, subtitle, body
        case recId = "rec_id"
        case targetPid = "target_pid"
        case targetName = "target_name"
        case rssMb = "rss_mb"
        case activeApp = "active_app"
    }
}

struct ServerMessage: Codable {
    let type: String
    let id: String?
    let timestampMs: Int64
    let text: String?
    let tool: String?
    let error: String?
    let telemetry: TelemetryPayload?
    let research: ResearchPayload?
    let notification: InactiveAppNotificationInfo?

    enum CodingKeys: String, CodingKey {
        case type, id, text, tool, error, telemetry, research, notification
        case timestampMs = "timestamp_ms"
    }
}

struct AssistantFollowUp: Identifiable, Hashable {
    let id = UUID()
    let label: String
    let prompt: String
    let icon: String
}

struct TranscriptMessage: Identifiable {
    let id = UUID()
    let isUser: Bool
    let text: String
    let timestamp: Date
    let tool: String?
    let followUps: [AssistantFollowUp]

    init(
        isUser: Bool,
        text: String,
        timestamp: Date,
        tool: String? = nil,
        followUps: [AssistantFollowUp] = []
    ) {
        self.isUser = isUser
        self.text = text
        self.timestamp = timestamp
        self.tool = tool
        self.followUps = followUps
    }
}

struct ActivityLine: Identifiable {
    let id = UUID()
    let timestamp: Date
    let text: String
    let count: Int

    init(timestamp: Date, text: String, count: Int = 1) {
        self.timestamp = timestamp
        self.text = text
        self.count = count
    }
}
