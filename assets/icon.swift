import AppKit
import CoreGraphics

let size: CGFloat = 1024
let space = CGColorSpace(name: CGColorSpace.sRGB)!
let ctx = CGContext(data: nil, width: Int(size), height: Int(size), bitsPerComponent: 8, bytesPerRow: 0, space: space, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
ctx.translateBy(x: 0, y: size)
ctx.scaleBy(x: 1, y: -1)

func color(_ hex: UInt32, _ a: CGFloat = 1) -> CGColor {
    CGColor(srgbRed: CGFloat((hex >> 16) & 0xff) / 255, green: CGFloat((hex >> 8) & 0xff) / 255, blue: CGFloat(hex & 0xff) / 255, alpha: a)
}

let tile = CGRect(x: 100, y: 100, width: 824, height: 824)
let tilePath = CGPath(roundedRect: tile, cornerWidth: 185, cornerHeight: 185, transform: nil)

ctx.saveGState()
ctx.setShadow(offset: CGSize(width: 0, height: 14), blur: 28, color: color(0x000000, 0.35))
ctx.addPath(tilePath)
ctx.setFillColor(color(0x4338CA))
ctx.fillPath()
ctx.restoreGState()

ctx.saveGState()
ctx.addPath(tilePath)
ctx.clip()
let bg = CGGradient(colorsSpace: space, colors: [color(0x6366F1), color(0x7C3AED), color(0x4C1D95)] as CFArray, locations: [0, 0.55, 1])!
ctx.drawLinearGradient(bg, start: CGPoint(x: 180, y: 100), end: CGPoint(x: 844, y: 924), options: [])
let glow = CGGradient(colorsSpace: space, colors: [color(0xFFFFFF, 0.22), color(0xFFFFFF, 0)] as CFArray, locations: [0, 1])!
ctx.drawRadialGradient(glow, startCenter: CGPoint(x: 360, y: 230), startRadius: 0, endCenter: CGPoint(x: 360, y: 230), endRadius: 520, options: [])
ctx.restoreGState()

let shield = CGMutablePath()
shield.move(to: CGPoint(x: 512, y: 238))
shield.addCurve(to: CGPoint(x: 752, y: 318), control1: CGPoint(x: 590, y: 292), control2: CGPoint(x: 680, y: 314))
shield.addLine(to: CGPoint(x: 752, y: 520))
shield.addCurve(to: CGPoint(x: 512, y: 822), control1: CGPoint(x: 752, y: 680), control2: CGPoint(x: 640, y: 770))
shield.addCurve(to: CGPoint(x: 272, y: 520), control1: CGPoint(x: 384, y: 770), control2: CGPoint(x: 272, y: 680))
shield.addLine(to: CGPoint(x: 272, y: 318))
shield.addCurve(to: CGPoint(x: 512, y: 238), control1: CGPoint(x: 344, y: 314), control2: CGPoint(x: 434, y: 292))
shield.closeSubpath()

ctx.saveGState()
ctx.setShadow(offset: CGSize(width: 0, height: 10), blur: 24, color: color(0x1E1B4B, 0.45))
ctx.addPath(shield)
ctx.setFillColor(color(0xFFFFFF))
ctx.fillPath()
ctx.restoreGState()

let barrier = CGPath(roundedRect: CGRect(x: 490, y: 515, width: 44, height: 180), cornerWidth: 22, cornerHeight: 22, transform: nil)
ctx.addPath(barrier)
ctx.setFillColor(color(0xA5B4FC))
ctx.fillPath()

let arrow = CGMutablePath()
arrow.move(to: CGPoint(x: 360, y: 640))
arrow.addCurve(to: CGPoint(x: 628, y: 590), control1: CGPoint(x: 380, y: 380), control2: CGPoint(x: 600, y: 380))
ctx.addPath(arrow)
ctx.setStrokeColor(color(0x5B21B6))
ctx.setLineWidth(52)
ctx.setLineCap(.round)
ctx.setLineJoin(.round)
ctx.strokePath()

let tip = CGPoint(x: 648, y: 660)
let head = CGMutablePath()
head.move(to: tip)
head.addLine(to: CGPoint(x: 560, y: 580))
head.addLine(to: CGPoint(x: 700, y: 548))
head.closeSubpath()
ctx.addPath(head)
ctx.setFillColor(color(0x5B21B6))
ctx.setLineWidth(18)
ctx.setStrokeColor(color(0x5B21B6))
ctx.setLineJoin(.round)
ctx.drawPath(using: .fillStroke)

let image = ctx.makeImage()!
let rep = NSBitmapImageRep(cgImage: image)
let out = CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : "icon.png"
try! rep.representation(using: .png, properties: [:])!.write(to: URL(fileURLWithPath: out))
