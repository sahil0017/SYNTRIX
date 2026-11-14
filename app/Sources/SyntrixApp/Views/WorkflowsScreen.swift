import AppKit
import SwiftUI

struct WorkflowsScreen: View {
    @ObservedObject var client: SyntrixClient

    private var hasPendingApproval: Bool {
        client.workflowRuns.contains { $0.status == "awaiting_approval" }
            || client.workflowOutput.localizedCaseInsensitiveContains("approval")
    }

    var body: some View {
        HStack(alignment: .top, spacing: 14) {
            // Left Column: Templates & Visual Pipeline Builder
            ScrollView {
                VStack(spacing: 12) {
                    HUDPanel(title: "WORKFLOW TEMPLATES", trailing: client.isWorkflowBusy ? "RUNNING..." : "TOOL REGISTRY + EVENT BUS") {
                        VStack(alignment: .leading, spacing: 8) {
                            workflowCard(
                                id: "mac-health-audit",
                                title: "Mac Health & Memory Audit",
                                badge: "POLICY-GATED",
                                badgeColor: HUDColors.amber,
                                detail: "C17 snapshot + OptimizationAgent + PolicyEngine approval."
                            )
                            workflowCard(
                                id: "cache-storage-audit",
                                title: "Cache & Storage Inspection",
                                badge: "READ-ONLY",
                                badgeColor: HUDColors.cyan,
                                detail: "Read-only scan of ~/Library/Caches & LaunchAgents."
                            )
                            workflowCard(
                                id: "dev-workspace-check",
                                title: "Developer Workspace Index",
                                badge: "INDEXING",
                                badgeColor: HUDColors.green,
                                detail: "Indexes docs/PDFs/code into SQLite + Vector RAG."
                            )
                        }
                    }

                    HUDPanel(title: "CUSTOM PIPELINE BUILDER", trailing: "\(client.customSteps.count) STEPS") {
                        VStack(alignment: .leading, spacing: 10) {
                            // Preset loader chips
                            ScrollView(.horizontal, showsIndicators: false) {
                                HStack(spacing: 6) {
                                    Text("PRESETS:")
                                        .font(.system(size: 9, weight: .bold, design: .monospaced))
                                        .foregroundStyle(HUDColors.textMuted)
                                    presetButton("Health Check") {
                                        client.customWorkflowName = "Health Check Pipeline"
                                        client.customSteps = [
                                            CustomWorkflowStep(name: "System Status", tool: "system.status", condition: "always", requireApproval: false, maxRetries: 1),
                                            CustomWorkflowStep(name: "Top Processes", tool: "system.top_processes", condition: "always", requireApproval: false, maxRetries: 1)
                                        ]
                                    }
                                    presetButton("Pressure Relief") {
                                        client.customWorkflowName = "Memory Pressure Relief"
                                        client.customSteps = [
                                            CustomWorkflowStep(name: "Check Status", tool: "system.status", condition: "always", requireApproval: false, maxRetries: 1),
                                            CustomWorkflowStep(name: "Opt Scan", tool: "optimization.scan", condition: "if_mem_gt:70", requireApproval: true, maxRetries: 1),
                                            CustomWorkflowStep(name: "Inspect Caches", tool: "storage.inspect_caches", condition: "if_disk_gt:50", requireApproval: true, maxRetries: 1)
                                        ]
                                    }
                                    presetButton("Security Audit") {
                                        client.customWorkflowName = "Mac Security Audit"
                                        client.customSteps = [
                                            CustomWorkflowStep(name: "Startup Agents", tool: "hardening.startup_agents", condition: "always", requireApproval: false, maxRetries: 1),
                                            CustomWorkflowStep(name: "Dev Inspect", tool: "developer.inspect", condition: "always", requireApproval: false, maxRetries: 1)
                                        ]
                                    }
                                }
                            }

                            HStack(spacing: 8) {
                                TextField("Workflow Name", text: $client.customWorkflowName)
                                    .textFieldStyle(.plain)
                                    .font(.system(size: 11, design: .monospaced))
                                    .foregroundStyle(HUDColors.textPrimary)
                                    .padding(6)
                                    .background(HUDColors.background)
                                    .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.cyanDim, lineWidth: 1))

                                Button("+ STEP") {
                                    client.customSteps.append(
                                        CustomWorkflowStep(name: "Step \(client.customSteps.count + 1)", tool: "system.top_processes", condition: "always", requireApproval: false, maxRetries: 1)
                                    )
                                }
                                .font(.system(size: 9, weight: .bold, design: .monospaced))
                                .foregroundStyle(HUDColors.cyan)
                                .padding(.horizontal, 7)
                                .padding(.vertical, 6)
                                .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.cyan, lineWidth: 1))
                                .buttonStyle(.plain)

                                Button("RUN") {
                                    client.runCustomWorkflow(approved: false)
                                }
                                .font(.system(size: 9, weight: .bold, design: .monospaced))
                                .foregroundStyle(HUDColors.green)
                                .padding(.horizontal, 8)
                                .padding(.vertical, 6)
                                .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.green, lineWidth: 1))
                                .buttonStyle(.plain)
                                .disabled(client.isWorkflowBusy || client.customSteps.isEmpty)
                            }

                            VStack(spacing: 6) {
                                ForEach(Array(client.customSteps.enumerated()), id: \.element.id) { index, _ in
                                    pipelineStepRow(index: index)
                                }
                            }
                        }
                    }
                }
            }
            .frame(maxWidth: 450)

            // Right Column: Execution Output & Persisted Runs
            VStack(spacing: 12) {
                HUDPanel(
                    title: "WORKFLOW EXECUTION OUTPUT",
                    trailing: client.isWorkflowBusy ? "EXECUTING \(client.activeWorkflowLabel.uppercased())..." : "EVENT BUS"
                ) {
                    VStack(alignment: .leading, spacing: 8) {
                        HStack(spacing: 8) {
                            if client.isWorkflowBusy {
                                ProgressView().scaleEffect(0.55).frame(width: 12, height: 12)
                                Text("Running workflow pipeline...")
                                    .font(.system(size: 10, design: .monospaced))
                                    .foregroundStyle(HUDColors.amber)
                            } else {
                                Text("Last run output")
                                    .font(.system(size: 10, design: .monospaced))
                                    .foregroundStyle(HUDColors.textMuted)
                            }
                            Spacer()
                            Button("Copy") {
                                NSPasteboard.general.clearContents()
                                NSPasteboard.general.setString(client.workflowOutput, forType: .string)
                            }
                            .font(.system(size: 9, weight: .semibold, design: .monospaced))
                            .foregroundStyle(HUDColors.cyan)
                            .buttonStyle(.plain)
                        }

                        if hasPendingApproval {
                            HStack(spacing: 10) {
                                Image(systemName: "hand.raised.fill")
                                    .foregroundStyle(HUDColors.amber)
                                Text("Approval gate paused execution.")
                                    .font(.system(size: 10, weight: .semibold, design: .monospaced))
                                    .foregroundStyle(HUDColors.amber)
                                Spacer()
                                Button("APPROVE & RESUME") {
                                    client.runCustomWorkflow(approved: true)
                                }
                                .font(.system(size: 9, weight: .bold, design: .monospaced))
                                .foregroundStyle(HUDColors.green)
                                .padding(.horizontal, 8)
                                .padding(.vertical, 4)
                                .background(HUDColors.green.opacity(0.14))
                                .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.green, lineWidth: 1))
                                .buttonStyle(.plain)
                            }
                            .padding(8)
                            .background(HUDColors.amber.opacity(0.10))
                            .overlay(RoundedRectangle(cornerRadius: 4).stroke(HUDColors.amber.opacity(0.45), lineWidth: 1))
                        }

                        ScrollView {
                            Text(client.workflowOutput)
                                .font(.system(size: 11, design: .monospaced))
                                .foregroundStyle(HUDColors.textPrimary)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .textSelection(.enabled)
                        }
                        .frame(maxHeight: .infinity)
                    }
                }

                HUDPanel(title: "PERSISTED WORKFLOW RUNS", trailing: "\(client.workflowRuns.count) RUNS") {
                    if client.workflowRuns.isEmpty {
                        Text("No persisted workflow runs yet. Run a template or custom pipeline above.")
                            .font(.system(size: 11, design: .monospaced))
                            .foregroundStyle(HUDColors.textMuted)
                    } else {
                        ScrollView {
                            VStack(alignment: .leading, spacing: 8) {
                                ForEach(client.workflowRuns) { run in
                                    let statusColor: Color = run.status == "completed" ? HUDColors.green : (run.status == "failed" ? HUDColors.red : HUDColors.amber)
                                    let progress = run.totalSteps > 0 ? Double(run.currentStep) / Double(run.totalSteps) : 0.0

                                    VStack(alignment: .leading, spacing: 4) {
                                        HStack(spacing: 8) {
                                            Text(run.status.uppercased())
                                                .font(.system(size: 8, weight: .bold, design: .monospaced))
                                                .foregroundStyle(statusColor)
                                                .padding(.horizontal, 5)
                                                .padding(.vertical, 2)
                                                .background(statusColor.opacity(0.14))
                                                .overlay(RoundedRectangle(cornerRadius: 2).stroke(statusColor.opacity(0.45), lineWidth: 1))

                                            Text(run.name)
                                                .font(.system(size: 10, weight: .semibold, design: .monospaced))
                                                .foregroundStyle(HUDColors.textPrimary)

                                            Text("(\(run.currentStep)/\(run.totalSteps))")
                                                .font(.system(size: 9, design: .monospaced))
                                                .foregroundStyle(HUDColors.textMuted)

                                            Spacer()

                                            if run.status == "awaiting_approval" || run.status == "failed" {
                                                Button(run.status == "awaiting_approval" ? "APPROVE & RESUME" : "RETRY") {
                                                    client.runCustomWorkflow(approved: true)
                                                }
                                                .font(.system(size: 9, weight: .bold, design: .monospaced))
                                                .foregroundStyle(HUDColors.green)
                                                .padding(.horizontal, 6)
                                                .padding(.vertical, 2)
                                                .overlay(RoundedRectangle(cornerRadius: 2).stroke(HUDColors.green, lineWidth: 1))
                                                .buttonStyle(.plain)
                                            }
                                        }

                                        GeometryReader { geo in
                                            ZStack(alignment: .leading) {
                                                RoundedRectangle(cornerRadius: 2).fill(HUDColors.cyanDim.opacity(0.25))
                                                RoundedRectangle(cornerRadius: 2)
                                                    .fill(statusColor)
                                                    .frame(width: max(4, geo.size.width * min(1.0, progress)))
                                            }
                                        }
                                        .frame(height: 4)
                                    }
                                    .padding(6)
                                    .background(statusColor.opacity(0.05))
                                    .overlay(RoundedRectangle(cornerRadius: 3).stroke(statusColor.opacity(0.25), lineWidth: 1))
                                }
                            }
                        }
                        .frame(maxHeight: 170)
                    }
                }
            }
        }
    }

    private func presetButton(_ title: String, action: @escaping () -> Void) -> some View {
        Button(title, action: action)
            .font(.system(size: 9, weight: .medium, design: .monospaced))
            .foregroundStyle(HUDColors.amber)
            .padding(.horizontal, 7)
            .padding(.vertical, 3)
            .background(HUDColors.panelBg)
            .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.amber.opacity(0.45), lineWidth: 1))
            .buttonStyle(.plain)
    }

    private func pipelineStepRow(index: Int) -> some View {
        HStack(spacing: 6) {
            Text("\(index + 1)")
                .font(.system(size: 9, weight: .bold, design: .monospaced))
                .foregroundStyle(HUDColors.cyan)
                .frame(width: 16, height: 16)
                .background(HUDColors.cyan.opacity(0.14))
                .cornerRadius(2)

            Picker("", selection: $client.customSteps[index].tool) {
                Text("system.status").tag("system.status")
                Text("system.top_processes").tag("system.top_processes")
                Text("optimization.scan").tag("optimization.scan")
                Text("storage.inspect_caches").tag("storage.inspect_caches")
                Text("hardening.startup_agents").tag("hardening.startup_agents")
                Text("developer.inspect").tag("developer.inspect")
            }
            .labelsHidden()
            .frame(maxWidth: 145)

            Picker("", selection: $client.customSteps[index].condition) {
                Text("always").tag("always")
                Text("mem > 70%").tag("if_mem_gt:70")
                Text("cpu > 75%").tag("if_cpu_gt:75")
                Text("disk > 50%").tag("if_disk_gt:50")
            }
            .labelsHidden()
            .frame(maxWidth: 95)

            Toggle("Gate", isOn: $client.customSteps[index].requireApproval)
                .font(.system(size: 9, design: .monospaced))
                .foregroundStyle(HUDColors.amber)
                .toggleStyle(.checkbox)

            Spacer(minLength: 0)

            Button("↑") {
                guard index > 0 else { return }
                client.customSteps.swapAt(index, index - 1)
            }
            .font(.system(size: 10, design: .monospaced))
            .foregroundStyle(index > 0 ? HUDColors.textPrimary : HUDColors.textMuted.opacity(0.4))
            .buttonStyle(.plain)
            .disabled(index == 0)

            Button("↓") {
                guard index + 1 < client.customSteps.count else { return }
                client.customSteps.swapAt(index, index + 1)
            }
            .font(.system(size: 10, design: .monospaced))
            .foregroundStyle(index + 1 < client.customSteps.count ? HUDColors.textPrimary : HUDColors.textMuted.opacity(0.4))
            .buttonStyle(.plain)
            .disabled(index + 1 >= client.customSteps.count)

            Button("×") {
                client.customSteps.remove(at: index)
            }
            .font(.system(size: 11, design: .monospaced))
            .foregroundStyle(HUDColors.red)
            .buttonStyle(.plain)
        }
        .padding(6)
        .background(HUDColors.cyanDim.opacity(0.10))
        .overlay(RoundedRectangle(cornerRadius: 3).stroke(HUDColors.cyanDim, lineWidth: 1))
    }

    private func workflowCard(id: String, title: String, badge: String, badgeColor: Color, detail: String) -> some View {
        let isRunning = client.isWorkflowBusy && client.activeWorkflowLabel == id
        return VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 6) {
                Text(title)
                    .font(.system(size: 11, weight: .semibold, design: .monospaced))
                    .foregroundStyle(HUDColors.cyan)
                Text(badge)
                    .font(.system(size: 8, weight: .bold, design: .monospaced))
                    .foregroundStyle(badgeColor)
                    .padding(.horizontal, 5)
                    .padding(.vertical, 1)
                    .background(badgeColor.opacity(0.12))
                    .overlay(RoundedRectangle(cornerRadius: 2).stroke(badgeColor.opacity(0.45), lineWidth: 1))
                Spacer()
                Button(isRunning ? "RUNNING" : "RUN") {
                    client.runWorkflow(id, dir: client.researchDir)
                }
                .font(.system(size: 10, weight: .bold, design: .monospaced))
                .foregroundStyle(isRunning ? HUDColors.amber : HUDColors.green)
                .padding(.horizontal, 8)
                .padding(.vertical, 3)
                .overlay(RoundedRectangle(cornerRadius: 3).stroke(isRunning ? HUDColors.amber : HUDColors.green, lineWidth: 1))
                .buttonStyle(.plain)
                .disabled(client.isWorkflowBusy)
            }
            Text(detail)
                .font(.system(size: 10, design: .monospaced))
                .foregroundStyle(HUDColors.textMuted)
        }
        .padding(8)
        .background(HUDColors.cyanDim.opacity(0.12))
        .overlay(RoundedRectangle(cornerRadius: 4).stroke(isRunning ? HUDColors.amber : Color.clear, lineWidth: 1))
        .cornerRadius(4)
    }
}
