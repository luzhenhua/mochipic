// Draw the Finder installer background and a compact document icon. Finder
// supplies the actual app and Applications icons on top of this artwork.
import AppKit
import CoreText
import Foundation
import ImageIO
import UniformTypeIdentifiers

struct InstallerCopy: Decodable {
    let titleBefore: String
    let titleAfter: String
    let helpHint: String
}

func color(_ hex: UInt32, alpha: CGFloat = 1) -> CGColor {
    CGColor(
        srgbRed: CGFloat((hex >> 16) & 0xff) / 255,
        green: CGFloat((hex >> 8) & 0xff) / 255,
        blue: CGFloat(hex & 0xff) / 255,
        alpha: alpha
    )
}

let accent = color(0x6D22E7)
let ink = color(0x201B30)

func canvas(_ width: Int, _ height: Int, scale: Int, draw: (CGContext) -> Void) -> CGImage {
    let ctx = CGContext(
        data: nil, width: width * scale, height: height * scale,
        bitsPerComponent: 8, bytesPerRow: 0, space: CGColorSpace(name: CGColorSpace.sRGB)!,
        bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
    )!
    ctx.translateBy(x: 0, y: CGFloat(height * scale))
    ctx.scaleBy(x: CGFloat(scale), y: -CGFloat(scale))
    draw(ctx)
    return ctx.makeImage()!
}

func save(_ image: CGImage, to path: URL, type: UTType, dpi: Int = 72) throws {
    let dest = CGImageDestinationCreateWithURL(path as CFURL, type.identifier as CFString, 1, nil)!
    CGImageDestinationAddImage(dest, image, [
        kCGImagePropertyDPIWidth: dpi,
        kCGImagePropertyDPIHeight: dpi,
    ] as CFDictionary)
    guard CGImageDestinationFinalize(dest) else { throw CocoaError(.fileWriteUnknown) }
}

func centered(
    _ parts: [(String, CGColor)], at center: CGPoint, size: CGFloat,
    weight: NSFont.Weight, maxWidth: CGFloat, in ctx: CGContext
) {
    func makeLine(_ pointSize: CGFloat) -> CTLine {
        let text = NSMutableAttributedString(string: "")
        for (string, fill) in parts {
            text.append(NSAttributedString(string: string, attributes: [
                .font: NSFont.systemFont(ofSize: pointSize, weight: weight),
                NSAttributedString.Key(kCTForegroundColorAttributeName as String): fill,
            ]))
        }
        return CTLineCreateWithAttributedString(text as CFAttributedString)
    }
    var line = makeLine(size)
    let initial = CTLineGetTypographicBounds(line, nil, nil, nil)
    if initial > maxWidth { line = makeLine(size * maxWidth / initial) }
    var ascent: CGFloat = 0
    var descent: CGFloat = 0
    let width = CTLineGetTypographicBounds(line, &ascent, &descent, nil)
    ctx.saveGState()
    ctx.translateBy(x: center.x - width / 2, y: center.y + (ascent - descent) / 2)
    ctx.scaleBy(x: 1, y: -1)
    ctx.textMatrix = .identity
    ctx.textPosition = .zero
    CTLineDraw(line, ctx)
    ctx.restoreGState()
}

func stroke(_ points: [CGPoint], width: CGFloat, fill: CGColor, in ctx: CGContext) {
    ctx.beginPath()
    ctx.addLines(between: points)
    ctx.setLineCap(.round)
    ctx.setLineJoin(.round)
    ctx.setStrokeColor(fill)
    ctx.setLineWidth(width)
    ctx.strokePath()
}

func background(_ copy: InstallerCopy, scale: Int) -> CGImage {
    canvas(676, 478, scale: scale) { ctx in
        let gradient = CGGradient(
            colorsSpace: CGColorSpace(name: CGColorSpace.sRGB),
            colors: [color(0xFCF9FF), color(0xF2ECFC)] as CFArray, locations: [0, 1]
        )!
        ctx.drawLinearGradient(gradient, start: CGPoint(x: 407, y: 0), end: CGPoint(x: 518, y: 510),
                               options: [.drawsBeforeStartLocation, .drawsAfterEndLocation])

        let upper = CGMutablePath()
        upper.move(to: CGPoint(x: 0, y: 0))
        upper.addLine(to: CGPoint(x: 153, y: 0))
        upper.addCurve(to: CGPoint(x: 77, y: 33), control1: CGPoint(x: 131, y: 44),
                       control2: CGPoint(x: 102, y: 24))
        upper.addCurve(to: CGPoint(x: 0, y: 87), control1: CGPoint(x: 42, y: 42),
                       control2: CGPoint(x: 39, y: 82))
        upper.closeSubpath()
        let lower = CGMutablePath()
        lower.move(to: CGPoint(x: 676, y: 350))
        lower.addCurve(to: CGPoint(x: 544, y: 450), control1: CGPoint(x: 609, y: 368),
                       control2: CGPoint(x: 621, y: 422))
        lower.addCurve(to: CGPoint(x: 487, y: 478), control1: CGPoint(x: 526, y: 456),
                       control2: CGPoint(x: 504, y: 464))
        lower.addLine(to: CGPoint(x: 676, y: 478))
        lower.closeSubpath()
        ctx.setFillColor(color(0x9D74EF, alpha: 0.15))
        ctx.addPath(upper)
        ctx.addPath(lower)
        ctx.fillPath()

        let spark = color(0x9B57EC)
        stroke([CGPoint(x: 321, y: 39), CGPoint(x: 326, y: 47)], width: 3.5, fill: spark, in: ctx)
        stroke([CGPoint(x: 338, y: 34), CGPoint(x: 338, y: 44)], width: 3.5, fill: spark, in: ctx)
        stroke([CGPoint(x: 355, y: 39), CGPoint(x: 350, y: 47)], width: 3.5, fill: spark, in: ctx)
        centered([(copy.titleBefore, ink), ("MochiPic", accent), (copy.titleAfter, ink)],
                 at: CGPoint(x: 338, y: 77), size: 31, weight: .heavy, maxWidth: 608, in: ctx)

        let arrow = color(0x968DAC)
        stroke([CGPoint(x: 307, y: 213), CGPoint(x: 366, y: 213)], width: 8, fill: arrow, in: ctx)
        stroke([CGPoint(x: 348, y: 195), CGPoint(x: 366, y: 213), CGPoint(x: 348, y: 231)],
               width: 8, fill: arrow, in: ctx)

        ctx.setFillColor(color(0xE9E0F7, alpha: 0.83))
        ctx.addPath(CGPath(roundedRect: CGRect(x: 148, y: 336, width: 380, height: 104),
                           cornerWidth: 20, cornerHeight: 20, transform: nil))
        ctx.fillPath()
        centered([(copy.helpHint, color(0x514462))], at: CGPoint(x: 338, y: 356),
                 size: 16, weight: .semibold, maxWidth: 328, in: ctx)
    }
}

func helpIcon() -> CGImage {
    // The file uses Finder's same icon cell as the two large items. Draw the
    // small document toward the lower edge, just above its real Finder label.
    canvas(512, 512, scale: 1) { ctx in
        let document = CGMutablePath()
        document.move(to: CGPoint(x: 231, y: 383))
        document.addLine(to: CGPoint(x: 260, y: 383))
        document.addLine(to: CGPoint(x: 280, y: 402))
        document.addLine(to: CGPoint(x: 280, y: 451))
        document.addQuadCurve(to: CGPoint(x: 275, y: 456), control: CGPoint(x: 280, y: 456))
        document.addLine(to: CGPoint(x: 231, y: 456))
        document.addQuadCurve(to: CGPoint(x: 226, y: 451), control: CGPoint(x: 226, y: 456))
        document.addLine(to: CGPoint(x: 226, y: 388))
        document.addQuadCurve(to: CGPoint(x: 231, y: 383), control: CGPoint(x: 226, y: 383))
        document.closeSubpath()
        ctx.addPath(document)
        ctx.setFillColor(color(0xFCF8FF))
        ctx.setStrokeColor(color(0x8365B5))
        ctx.setLineWidth(5.5)
        ctx.drawPath(using: .fillStroke)
        stroke([CGPoint(x: 260, y: 384), CGPoint(x: 260, y: 403), CGPoint(x: 279, y: 403)],
               width: 5, fill: color(0x8365B5), in: ctx)
        for (y, end) in [(415.0, 266.0), (427.0, 266.0), (439.0, 259.0)] {
            stroke([CGPoint(x: 239, y: y), CGPoint(x: end, y: y)],
                   width: 4.5, fill: color(0x9B81C8), in: ctx)
        }
    }
}

let args = CommandLine.arguments
guard args.count == 4 else { throw CocoaError(.fileReadInvalidFileName) }
if args[1] == "--set-help-icon" {
    guard let icon = NSImage(contentsOfFile: args[2]),
          NSWorkspace.shared.setIcon(icon, forFile: args[3], options: []) else {
        throw CocoaError(.fileWriteUnknown)
    }
} else {
    let copies = try JSONDecoder().decode([String: InstallerCopy].self, from: Data(contentsOf: URL(fileURLWithPath: args[1])))
    guard let copy = copies[args[2]] else { throw CocoaError(.fileReadCorruptFile) }
    let folder = URL(fileURLWithPath: args[3], isDirectory: true)
    try save(background(copy, scale: 1), to: folder.appendingPathComponent("background.tiff"), type: .tiff)
    try save(background(copy, scale: 2), to: folder.appendingPathComponent("background@2x.tiff"), type: .tiff, dpi: 144)
    try save(helpIcon(), to: folder.appendingPathComponent("help.png"), type: .png)
}
