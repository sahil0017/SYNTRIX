import SwiftUI

enum HUDColors {
    static let background = Color(red: 0.016, green: 0.039, blue: 0.063)
    static let panelBg = Color(red: 0.027, green: 0.063, blue: 0.098).opacity(0.85)
    static let cyan = Color(red: 0.149, green: 0.776, blue: 0.894)
    static let cyanDim = Color(red: 0.149, green: 0.776, blue: 0.894).opacity(0.28)
    static let amber = Color(red: 0.945, green: 0.678, blue: 0.255)
    static let red = Color(red: 0.95, green: 0.30, blue: 0.32)
    static let green = Color(red: 0.20, green: 0.84, blue: 0.56)
    static let textPrimary = Color(red: 0.86, green: 0.94, blue: 0.98)
    static let textMuted = Color(red: 0.45, green: 0.62, blue: 0.72)
}

struct HUDPanel<Content: View>: View {
    let title: String
    var trailing: String? = nil
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text(title)
                    .font(.system(size: 11, weight: .semibold, design: .monospaced))
                    .tracking(2.2)
                    .foregroundStyle(HUDColors.cyan)
                Spacer()
                if let trailing {
                    Text(trailing)
                        .font(.system(size: 10, weight: .medium, design: .monospaced))
                        .foregroundStyle(HUDColors.textMuted)
                }
            }
            Divider()
                .overlay(HUDColors.cyanDim)
            content
        }
        .padding(12)
        .background(HUDColors.panelBg)
        .overlay(
            RoundedRectangle(cornerRadius: 4)
                .stroke(HUDColors.cyanDim, lineWidth: 1)
        )
        .overlay(HUDCorners())
    }
}

struct HUDCorners: View {
    var body: some View {
        GeometryReader { geo in
            let w = geo.size.width
            let h = geo.size.height
            let len: CGFloat = 10
            Path { p in
                // Top-left
                p.move(to: CGPoint(x: 0, y: len))
                p.addLine(to: CGPoint(x: 0, y: 0))
                p.addLine(to: CGPoint(x: len, y: 0))
                // Top-right
                p.move(to: CGPoint(x: w - len, y: 0))
                p.addLine(to: CGPoint(x: w, y: 0))
                p.addLine(to: CGPoint(x: w, y: len))
                // Bottom-left
                p.move(to: CGPoint(x: 0, y: h - len))
                p.addLine(to: CGPoint(x: 0, y: h))
                p.addLine(to: CGPoint(x: len, y: h))
                // Bottom-right
                p.move(to: CGPoint(x: w - len, y: h))
                p.addLine(to: CGPoint(x: w, y: h))
                p.addLine(to: CGPoint(x: w, y: h - len))
            }
            .stroke(HUDColors.cyan.opacity(0.85), lineWidth: 1.5)
        }
        .allowsHitTesting(false)
    }
}

struct HUDProgressBar: View {
    let label: String
    let valuePct: Double
    var accent: Color = HUDColors.amber

    var body: some View {
        HStack(spacing: 8) {
            Text(label)
                .font(.system(size: 11, weight: .medium, design: .monospaced))
                .foregroundStyle(HUDColors.textMuted)
                .frame(width: 32, alignment: .leading)

            GeometryReader { geo in
                let clamped = max(0, min(100, valuePct))
                let fillWidth = geo.size.width * (clamped / 100.0)
                ZStack(alignment: .leading) {
                    Capsule()
                        .fill(HUDColors.cyanDim.opacity(0.4))
                    Capsule()
                        .fill(accent)
                        .frame(width: max(4, fillWidth))
                        .shadow(color: accent.opacity(0.6), radius: 4)
                }
            }
            .frame(height: 8)

            Text(String(format: "%3.0f%%", valuePct))
                .font(.system(size: 11, weight: .semibold, design: .monospaced))
                .foregroundStyle(HUDColors.textPrimary)
                .frame(width: 38, alignment: .trailing)
        }
    }
}
