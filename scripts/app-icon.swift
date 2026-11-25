// Render a macOS icon set or a transparent PNG tile for app surfaces.
import CoreGraphics
import Foundation
import ImageIO
import UniformTypeIdentifiers

let arguments = CommandLine.arguments
guard arguments.count == 3 || arguments.count == 4,
      let file = CGImageSourceCreateWithURL(URL(fileURLWithPath: arguments[1]) as CFURL, nil),
      let image = CGImageSourceCreateImageAtIndex(file, 0, nil),
      image.width == image.height else {
    throw CocoaError(.fileReadCorruptFile)
}
let target = URL(fileURLWithPath: arguments[2], isDirectory: arguments.count == 3)
let cornerRatio: CGFloat = 204.0 / 864.0

func render(_ px: Int, to url: URL, inset: CGFloat) throws {
    let space = CGColorSpace(name: CGColorSpace.sRGB)!
    let ctx = CGContext(data: nil, width: px, height: px, bitsPerComponent: 8,
                        bytesPerRow: px * 4, space: space,
                        bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
    ctx.scaleBy(x: CGFloat(px) / 1024, y: CGFloat(px) / 1024)
    let tile = CGRect(x: 0, y: 0, width: 1024, height: 1024).insetBy(dx: inset, dy: inset)
    let radius = tile.width * cornerRatio
    ctx.addPath(CGPath(roundedRect: tile, cornerWidth: radius, cornerHeight: radius, transform: nil))
    ctx.clip()
    ctx.interpolationQuality = .high
    ctx.draw(image, in: tile)

    let out = CGImageDestinationCreateWithURL(url as CFURL, UTType.png.identifier as CFString, 1, nil)!
    CGImageDestinationAddImage(out, ctx.makeImage()!, nil)
    if !CGImageDestinationFinalize(out) {
        throw CocoaError(.fileWriteUnknown)
    }
}

if arguments.count == 4 {
    guard let size = Int(arguments[3]), (16...1024).contains(size), target.pathExtension == "png" else {
        throw CocoaError(.fileWriteInvalidFileName)
    }
    try render(size, to: target, inset: 0)
} else {
    try FileManager.default.createDirectory(at: target, withIntermediateDirectories: true)
    for size in [16, 32, 128, 256, 512] {
        try render(size, to: target.appendingPathComponent("icon_\(size)x\(size).png"), inset: 80)
        try render(size * 2, to: target.appendingPathComponent("icon_\(size)x\(size)@2x.png"), inset: 80)
    }
}
