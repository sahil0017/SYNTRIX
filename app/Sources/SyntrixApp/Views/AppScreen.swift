import SwiftUI

enum AppScreen: String, CaseIterable, Identifiable {
    case overview = "Overview"
    case assistant = "Assistant"
    case telemetry = "Telemetry"
    case research = "Research"
    case workflows = "Workflows"
    case security = "Security"

    var id: String { rawValue }

    var icon: String {
        switch self {
        case .overview: return "gauge.with.dots.needle.33percent"
        case .assistant: return "bubble.left.and.bubble.right"
        case .telemetry: return "cpu"
        case .research: return "doc.text.magnifyingglass"
        case .workflows: return "flowchart"
        case .security: return "shield.lefthalf.filled"
        }
    }

    var subtitle: String {
        switch self {
        case .overview: return "Live Mac health, predictive diagnosis & quick actions"
        case .assistant: return "Offline AI companion, developer & learning agents"
        case .telemetry: return "Hardware metrics, top processes & resource governor"
        case .research: return "SQLite vector RAG, PDF/code indexing & learning paths"
        case .workflows: return "Built-in & conditional multi-step automations"
        case .security: return "macOS TCC permissions, context capture & audit trail"
        }
    }
}

enum HUDFormatters {
    static let time: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "HH:mm:ss"
        return f
    }()

    static let date: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "EEE dd MMM yyyy"
        return f
    }()

    static func rate(_ bytesPerSec: Double) -> String {
        if bytesPerSec >= 1024 * 1024 {
            return String(format: "%.1f MB/s", bytesPerSec / (1024 * 1024))
        }
        return String(format: "%.0f KB/s", bytesPerSec / 1024)
    }

    static func bytesGB(_ bytes: UInt64) -> String {
        String(format: "%.2f GB", Double(bytes) / (1024 * 1024 * 1024))
    }

    static func bytesMB(_ bytes: UInt64) -> String {
        String(format: "%.0f MB", Double(bytes) / (1024 * 1024))
    }
}
