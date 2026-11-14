import SwiftUI

struct SyntrixCoreView: View {
    let cpuPct: Double
    let memoryPressurePct: Double
    let mode: OperatingMode
    let isConnected: Bool

    var body: some View {
        VStack(spacing: 12) {
            TimelineView(.animation(minimumInterval: 1.0 / 30.0)) { timeline in
                let t = timeline.date.timeIntervalSinceReferenceDate
                let speed = mode == .off ? 0.08 : (0.35 + (cpuPct / 100.0) * 1.2)

                Canvas { ctx, size in
                    let center = CGPoint(x: size.width / 2, y: size.height / 2)
                    let maxR = min(size.width, size.height) * 0.45

                    // Subtle outer tick ring
                    let tickCount = 60
                    for i in 0..<tickCount {
                        let angle = (Double(i) / Double(tickCount)) * 2 * .pi + (t * speed * 0.15)
                        let r1 = maxR * 0.94
                        let r2 = (i % 5 == 0) ? maxR * 1.0 : maxR * 0.97
                        var tick = Path()
                        tick.move(to: point(center, r1, angle))
                        tick.addLine(to: point(center, r2, angle))
                        ctx.stroke(
                            tick,
                            with: .color(HUDColors.cyan.opacity(i % 5 == 0 ? 0.7 : 0.3)),
                            lineWidth: i % 5 == 0 ? 1.5 : 1.0
                        )
                    }

                    // Segmented cyan ring
                    let segCount = 24
                    for i in 0..<segCount {
                        let start = (Double(i) / Double(segCount)) * 2 * .pi - (t * speed * 0.25)
                        let end = start + (2 * .pi / Double(segCount)) * 0.55
                        var arc = Path()
                        arc.addArc(center: center, radius: maxR * 0.80, startAngle: .radians(start), endAngle: .radians(end), clockwise: false)
                        ctx.stroke(arc, with: .color(HUDColors.cyan.opacity(mode == .off ? 0.25 : 0.75)), lineWidth: 5)
                    }

                    // Rotating amber arc reflecting CPU load
                    let loadSpan = max(0.25, min(1.8 * .pi, (cpuPct / 100.0) * 1.8 * .pi))
                    let amberStart = t * speed * 0.6
                    var amberArc = Path()
                    amberArc.addArc(center: center, radius: maxR * 0.89, startAngle: .radians(amberStart), endAngle: .radians(amberStart + loadSpan), clockwise: false)
                    ctx.stroke(amberArc, with: .color(HUDColors.amber.opacity(mode == .off ? 0.2 : 0.9)), lineWidth: 2.5)

                    // Inner hexagon
                    let hexR = maxR * 0.52
                    var hex = Path()
                    for i in 0..<6 {
                        let angle = (Double(i) / 6.0) * 2 * .pi + (t * speed * 0.2)
                        let pt = point(center, hexR, angle)
                        if i == 0 { hex.move(to: pt) } else { hex.addLine(to: pt) }
                    }
                    hex.closeSubpath()
                    ctx.stroke(hex, with: .color(HUDColors.cyan.opacity(0.65)), lineWidth: 1.5)

                    // Inner core glow
                    let coreR = maxR * (0.20 + 0.02 * sin(t * 2.5))
                    let glowRect = CGRect(x: center.x - coreR * 1.8, y: center.y - coreR * 1.8, width: coreR * 3.6, height: coreR * 3.6)
                    let coreColor = mode == .off ? HUDColors.textMuted : (memoryPressurePct >= 80 ? HUDColors.amber : HUDColors.cyan)
                    ctx.fill(
                        Path(ellipseIn: glowRect),
                        with: .radialGradient(
                            Gradient(colors: [coreColor.opacity(0.75), coreColor.opacity(0.0)]),
                            center: center,
                            startRadius: 2,
                            endRadius: coreR * 1.8
                        )
                    )
                    let solidRect = CGRect(x: center.x - coreR * 0.6, y: center.y - coreR * 0.6, width: coreR * 1.2, height: coreR * 1.2)
                    ctx.fill(Path(ellipseIn: solidRect), with: .color(Color.white.opacity(mode == .off ? 0.3 : 0.9)))
                }
            }
            .frame(minWidth: 240, maxWidth: 320, minHeight: 240, maxHeight: 320)

            Text(isConnected ? (mode == .off ? "STANDBY" : "ONLINE · \(mode.rawValue)") : "CONNECTING...")
                .font(.system(size: 12, weight: .semibold, design: .monospaced))
                .tracking(3.0)
                .foregroundStyle(isConnected ? HUDColors.cyan : HUDColors.amber)
        }
    }

    private func point(_ center: CGPoint, _ radius: CGFloat, _ angle: Double) -> CGPoint {
        CGPoint(
            x: center.x + radius * cos(angle),
            y: center.y + radius * sin(angle)
        )
    }
}
