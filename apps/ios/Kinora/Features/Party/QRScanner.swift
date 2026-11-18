import SwiftUI
import AVFoundation
import UIKit

/// Live camera preview that reports QR codes. iPhone and iPad only.
///
/// `onCode` gets every QR string the camera sees (the caller decides what it is and stops
/// listening with `paused` once it has what it needs). The camera runs only while the view is
/// on screen and not paused.
struct QRScanner: UIViewRepresentable {
    var paused: Bool
    var torch: Bool
    var onCode: (String) -> Void

    /// Whether this device has a back camera at all (not the simulator, not every iPad).
    static var available: Bool { AVCaptureDevice.default(for: .video) != nil }

    func makeUIView(context: Context) -> ScannerView {
        let v = ScannerView()
        v.onCode = onCode
        return v
    }

    func updateUIView(_ v: ScannerView, context: Context) {
        v.onCode = onCode
        v.setRunning(!paused)
        v.setTorch(torch && !paused)
    }

    static func dismantleUIView(_ v: ScannerView, coordinator: ()) {
        v.setTorch(false)
        v.setRunning(false)
    }

    final class ScannerView: UIView, AVCaptureMetadataOutputObjectsDelegate {
        var onCode: ((String) -> Void)?
        private let session = AVCaptureSession()
        private let device = AVCaptureDevice.default(for: .video)
        private var configured = false
        private var running = false

        override class var layerClass: AnyClass { AVCaptureVideoPreviewLayer.self }
        private var preview: AVCaptureVideoPreviewLayer { layer as! AVCaptureVideoPreviewLayer }

        override init(frame: CGRect) {
            super.init(frame: frame)
            backgroundColor = .black
            preview.videoGravity = .resizeAspectFill
            preview.session = session
        }

        required init?(coder: NSCoder) { fatalError("init(coder:) is not used") }

        private func configure() -> Bool {
            if configured { return true }
            guard let device, let input = try? AVCaptureDeviceInput(device: device),
                  session.canAddInput(input) else { return false }
            session.beginConfiguration()
            session.addInput(input)
            let output = AVCaptureMetadataOutput()
            if session.canAddOutput(output) {
                session.addOutput(output)
                output.setMetadataObjectsDelegate(self, queue: .main)
                if output.availableMetadataObjectTypes.contains(.qr) { output.metadataObjectTypes = [.qr] }
            }
            session.commitConfiguration()
            // Close-up codes on a phone screen: keep the focus near.
            if (try? device.lockForConfiguration()) != nil {
                if device.isAutoFocusRangeRestrictionSupported { device.autoFocusRangeRestriction = .near }
                device.unlockForConfiguration()
            }
            configured = true
            return true
        }

        func setRunning(_ on: Bool) {
            guard on != running else { return }
            if on, !configure() { return }
            running = on
            // startRunning blocks for a moment: off the main thread.
            let box = SessionBox(session: session)
            DispatchQueue.global(qos: .userInitiated).async {
                if on { box.session.startRunning() } else { box.session.stopRunning() }
            }
        }

        func setTorch(_ on: Bool) {
            guard let device, device.hasTorch, configured else { return }
            guard (device.torchMode == .on) != on, (try? device.lockForConfiguration()) != nil else { return }
            device.torchMode = on ? .on : .off
            device.unlockForConfiguration()
        }

        nonisolated func metadataOutput(_ output: AVCaptureMetadataOutput, didOutput objects: [AVMetadataObject],
                                        from connection: AVCaptureConnection) {
            let codes = objects.compactMap { ($0 as? AVMetadataMachineReadableCodeObject)?.stringValue }
            guard !codes.isEmpty else { return }
            MainActor.assumeIsolated {
                guard running else { return }
                codes.forEach { onCode?($0) }
            }
        }
    }

    private struct SessionBox: @unchecked Sendable { let session: AVCaptureSession }

    static var hasTorch: Bool { AVCaptureDevice.default(for: .video)?.hasTorch ?? false }
}

/// Camera permission, as the join screen needs it.
enum CameraAccess {
    case allowed, undecided, denied

    static var current: CameraAccess {
        switch AVCaptureDevice.authorizationStatus(for: .video) {
        case .authorized: .allowed
        case .notDetermined: .undecided
        default: .denied
        }
    }

    static func request() async -> CameraAccess {
        await AVCaptureDevice.requestAccess(for: .video) ? .allowed : .denied
    }
}
