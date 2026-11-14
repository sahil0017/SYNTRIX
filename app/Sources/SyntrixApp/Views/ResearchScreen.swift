import AppKit
import SwiftUI

struct ResearchScreen: View {
    @ObservedObject var client: SyntrixClient

    private let suggestedQueries = ["TaskPlanner", "predictive_model", "optimizer", "sqlite", "governor", "benchmark"]

    var body: some View {
        VStack(spacing: 12) {
            statsHeaderBar

            HUDPanel(title: "RESEARCH & KNOWLEDGE BASE", trailing: client.isResearchBusy ? "WORKING..." : "HYBRID VECTOR + LEXICAL RAG") {
                VStack(alignment: .leading, spacing: 10) {
                    // Directory indexing row
                    HStack(spacing: 10) {
                        TextField("Workspace directory to index...", text: $client.researchDir)
                            .textFieldStyle(.plain)
                            .font(.system(size: 11, design: .monospaced))
                            .foregroundStyle(HUDColors.textPrimary)
                            .padding(8)
                            .background(HUDColors.background)
                            .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.cyanDim, lineWidth: 1))

                        Button {
                            client.pickResearchDirectory()
                        } label: {
                            Label("BROWSE", systemImage: "folder")
                                .font(.system(size: 10, weight: .semibold, design: .monospaced))
                                .foregroundStyle(HUDColors.textPrimary)
                                .padding(.horizontal, 10)
                                .padding(.vertical, 7)
                                .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.cyanDim, lineWidth: 1))
                        }
                        .buttonStyle(.plain)

                        Button {
                            client.indexDirectory(client.researchDir)
                        } label: {
                            HStack(spacing: 5) {
                                if client.isResearchBusy {
                                    ProgressView().scaleEffect(0.55).frame(width: 10, height: 10)
                                } else {
                                    Image(systemName: "arrow.triangle.2.circlepath")
                                }
                                Text("INDEX DIR")
                            }
                            .font(.system(size: 10, weight: .semibold, design: .monospaced))
                            .foregroundStyle(HUDColors.cyan)
                            .padding(.horizontal, 10)
                            .padding(.vertical, 7)
                            .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.cyan, lineWidth: 1))
                        }
                        .buttonStyle(.plain)
                        .disabled(client.isResearchBusy)
                    }

                    // Search & Learn row
                    HStack(spacing: 10) {
                        TextField("Search indexed PDFs, Markdown, Go, Swift, Python & shell scripts...", text: $client.researchQuery)
                            .textFieldStyle(.plain)
                            .font(.system(size: 11, design: .monospaced))
                            .foregroundStyle(HUDColors.textPrimary)
                            .padding(8)
                            .background(HUDColors.background)
                            .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.cyanDim, lineWidth: 1))
                            .onSubmit {
                                client.searchKnowledge(client.researchQuery)
                            }

                        Button {
                            client.searchKnowledge(client.researchQuery)
                        } label: {
                            Label("SEARCH", systemImage: "magnifyingglass")
                                .font(.system(size: 10, weight: .semibold, design: .monospaced))
                                .foregroundStyle(HUDColors.amber)
                                .padding(.horizontal, 10)
                                .padding(.vertical, 7)
                                .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.amber, lineWidth: 1))
                        }
                        .buttonStyle(.plain)
                        .disabled(client.isResearchBusy)

                        Button {
                            let topic = client.researchQuery.isEmpty ? "SYNTRIX Architecture" : client.researchQuery
                            client.sendPrompt("/learn \(topic)")
                            client.activeTab = AppScreen.assistant.rawValue
                        } label: {
                            Label("LEARN PATH", systemImage: "graduationcap")
                                .font(.system(size: 10, weight: .semibold, design: .monospaced))
                                .foregroundStyle(HUDColors.green)
                                .padding(.horizontal, 10)
                                .padding(.vertical, 7)
                                .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.green, lineWidth: 1))
                        }
                        .buttonStyle(.plain)
                    }

                    // Dynamic concepts or suggested queries
                    conceptAndQueryStrip

                    // View mode switcher
                    HStack(spacing: 8) {
                        ForEach(["HITS", "GRAPH", "RAW"], id: \.self) { tab in
                            let active = client.researchTab == tab
                            Button(tabLabel(tab)) {
                                client.researchTab = tab
                            }
                            .font(.system(size: 9, weight: .bold, design: .monospaced))
                            .foregroundStyle(active ? HUDColors.cyan : HUDColors.textMuted)
                            .padding(.horizontal, 8)
                            .padding(.vertical, 4)
                            .background(active ? HUDColors.cyan.opacity(0.14) : Color.clear)
                            .overlay(RoundedRectangle(cornerRadius: 3).stroke(active ? HUDColors.cyan : HUDColors.cyanDim, lineWidth: 1))
                            .buttonStyle(.plain)
                        }
                        Spacer()
                        Text(client.researchOutput.split(separator: "\n").first.map(String.init) ?? "")
                            .font(.system(size: 9, design: .monospaced))
                            .foregroundStyle(HUDColors.textMuted)
                            .lineLimit(1)
                    }

                    Divider().overlay(HUDColors.cyanDim)

                    ScrollView {
                        switch client.researchTab {
                        case "GRAPH":
                            graphView
                        case "RAW":
                            Text(client.researchOutput)
                                .font(.system(size: 11, design: .monospaced))
                                .foregroundStyle(HUDColors.textPrimary)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .textSelection(.enabled)
                        default:
                            hitsView
                        }
                    }
                    .frame(maxHeight: .infinity)
                }
            }
            .frame(maxHeight: .infinity)
        }
    }

    private var statsHeaderBar: some View {
        HStack(spacing: 10) {
            statBadge("DOCS", "\(client.researchStats?.docCount ?? 0)", HUDColors.cyan)
            statBadge("CHUNKS", "\(client.researchStats?.chunkCount ?? 0)", HUDColors.amber)
            statBadge("GRAPH EDGES", "\(client.researchStats?.edgeCount ?? 0)", HUDColors.green)
            Spacer()
            if client.researchStats == nil {
                Button("Quick Index SYNTRIX") {
                    client.indexDirectory(client.researchDir)
                }
                .font(.system(size: 10, weight: .semibold, design: .monospaced))
                .foregroundStyle(HUDColors.cyan)
                .padding(.horizontal, 9)
                .padding(.vertical, 4)
                .background(HUDColors.cyan.opacity(0.12))
                .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.cyan, lineWidth: 1))
                .buttonStyle(.plain)
            }
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 7)
        .background(HUDColors.panelBg)
        .overlay(RoundedRectangle(cornerRadius: 4).stroke(HUDColors.cyanDim, lineWidth: 1))
    }

    private func statBadge(_ label: String, _ value: String, _ color: Color) -> some View {
        HStack(spacing: 5) {
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

    private var conceptAndQueryStrip: some View {
        let concepts = client.researchStats?.topConcepts ?? []
        let chips = concepts.isEmpty ? suggestedQueries : concepts

        return ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 6) {
                Text(concepts.isEmpty ? "TRY:" : "CONCEPTS:")
                    .font(.system(size: 9, weight: .bold, design: .monospaced))
                    .foregroundStyle(HUDColors.textMuted)

                ForEach(chips, id: \.self) { chip in
                    Button(chip) {
                        client.searchKnowledge(chip)
                    }
                    .font(.system(size: 9, weight: .medium, design: .monospaced))
                    .foregroundStyle(concepts.isEmpty ? HUDColors.amber : HUDColors.cyan)
                    .padding(.horizontal, 7)
                    .padding(.vertical, 3)
                    .background(HUDColors.panelBg)
                    .overlay(RoundedRectangle(cornerRadius: 3).stroke((concepts.isEmpty ? HUDColors.amber : HUDColors.cyan).opacity(0.4), lineWidth: 1))
                    .buttonStyle(.plain)
                }
            }
        }
    }

    private func tabLabel(_ tab: String) -> String {
        switch tab {
        case "HITS":
            let count = client.researchStats?.hits?.count ?? 0
            return count > 0 ? "SEMANTIC HITS (\(count))" : "SEMANTIC HITS"
        case "GRAPH":
            let count = client.researchStats?.graphEdges?.count ?? 0
            return count > 0 ? "KNOWLEDGE GRAPH (\(count))" : "KNOWLEDGE GRAPH"
        default:
            return "RAW OUTPUT"
        }
    }

    @ViewBuilder
    private var hitsView: some View {
        let hits = client.researchStats?.hits ?? []
        if hits.isEmpty {
            VStack(alignment: .leading, spacing: 8) {
                Text(client.researchOutput)
                    .font(.system(size: 11, design: .monospaced))
                    .foregroundStyle(HUDColors.textPrimary)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .textSelection(.enabled)
            }
        } else {
            VStack(spacing: 8) {
                ForEach(hits) { hit in
                    VStack(alignment: .leading, spacing: 6) {
                        HStack(spacing: 8) {
                            Text(String(format: "%.0f%%", hit.score * 100.0))
                                .font(.system(size: 9, weight: .bold, design: .monospaced))
                                .foregroundStyle(hit.score >= 0.45 ? HUDColors.green : HUDColors.amber)
                                .padding(.horizontal, 5)
                                .padding(.vertical, 2)
                                .background((hit.score >= 0.45 ? HUDColors.green : HUDColors.amber).opacity(0.14))
                                .overlay(RoundedRectangle(cornerRadius: 2).stroke((hit.score >= 0.45 ? HUDColors.green : HUDColors.amber).opacity(0.45), lineWidth: 1))

                            Text(hit.title)
                                .font(.system(size: 11, weight: .bold, design: .monospaced))
                                .foregroundStyle(HUDColors.cyan)

                            Text(hit.docPath)
                                .font(.system(size: 9, design: .monospaced))
                                .foregroundStyle(HUDColors.textMuted)
                                .lineLimit(1)
                                .truncationMode(.middle)

                            Spacer()

                            Button("Copy Path") {
                                NSPasteboard.general.clearContents()
                                NSPasteboard.general.setString(hit.docPath, forType: .string)
                            }
                            .font(.system(size: 8, weight: .semibold, design: .monospaced))
                            .foregroundStyle(HUDColors.textMuted)
                            .buttonStyle(.plain)

                            Button("Learn") {
                                client.sendPrompt("/learn \(hit.title)")
                                client.activeTab = AppScreen.assistant.rawValue
                            }
                            .font(.system(size: 8, weight: .semibold, design: .monospaced))
                            .foregroundStyle(HUDColors.green)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .overlay(RoundedRectangle(cornerRadius: 2).stroke(HUDColors.green.opacity(0.5), lineWidth: 1))
                            .buttonStyle(.plain)
                        }

                        Text(hit.snippet.trimmingCharacters(in: .whitespacesAndNewlines))
                            .font(.system(size: 10, design: .monospaced))
                            .foregroundStyle(HUDColors.textPrimary)
                            .lineLimit(5)
                            .textSelection(.enabled)
                    }
                    .padding(9)
                    .background(HUDColors.cyan.opacity(0.05))
                    .overlay(RoundedRectangle(cornerRadius: 4).stroke(HUDColors.cyanDim, lineWidth: 1))
                }
            }
        }
    }

    @ViewBuilder
    private var graphView: some View {
        let edges = client.researchStats?.graphEdges ?? []
        if edges.isEmpty {
            Text("Index a workspace directory to build the local concept knowledge graph.")
                .font(.system(size: 11, design: .monospaced))
                .foregroundStyle(HUDColors.textMuted)
                .frame(maxWidth: .infinity, alignment: .leading)
        } else {
            LazyVGrid(columns: [GridItem(.flexible()), GridItem(.flexible())], spacing: 8) {
                ForEach(edges) { edge in
                    Button {
                        client.searchKnowledge(edge.to)
                        client.researchTab = "HITS"
                    } label: {
                        HStack(spacing: 6) {
                            Text(edge.from)
                                .font(.system(size: 10, weight: .semibold, design: .monospaced))
                                .foregroundStyle(HUDColors.textPrimary)
                                .lineLimit(1)
                            Text("─[\(edge.kind)]→")
                                .font(.system(size: 9, design: .monospaced))
                                .foregroundStyle(HUDColors.textMuted)
                            Text(edge.to)
                                .font(.system(size: 10, weight: .bold, design: .monospaced))
                                .foregroundStyle(HUDColors.cyan)
                                .lineLimit(1)
                            Spacer()
                        }
                        .padding(8)
                        .background(HUDColors.panelBg)
                        .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.cyanDim, lineWidth: 1))
                    }
                    .buttonStyle(.plain)
                }
            }
        }
    }
}
