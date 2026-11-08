import DeviceCheck
import Flutter
import UIKit

@main
@objc class AppDelegate: FlutterAppDelegate, FlutterImplicitEngineDelegate {
  override func application(
    _ application: UIApplication,
    didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
  ) -> Bool {
    return super.application(application, didFinishLaunchingWithOptions: launchOptions)
  }

  func didInitializeImplicitFlutterEngine(_ engineBridge: FlutterImplicitEngineBridge) {
    GeneratedPluginRegistrant.register(with: engineBridge.pluginRegistry)
    registerAttestChannel(engineBridge.pluginRegistry)
  }

  // Native side of the nexus/attest platform channel: mints a DeviceCheck
  // token (32 bytes, base64) that the Go API verifies against Apple's
  // validate_device_token endpoint. The nonce is bound server-side.
  private func registerAttestChannel(_ registry: FlutterPluginRegistry) {
    guard let registrar = registry.registrar(forPlugin: "NexusAttest") else { return }
    let channel = FlutterMethodChannel(
      name: "nexus/attest",
      binaryMessenger: registrar.messenger()
    )
    channel.setMethodCallHandler { call, result in
      guard call.method == "getPlatformToken" else {
        result(FlutterMethodNotImplemented)
        return
      }
      guard DCDevice.current.isSupported else {
        result(
          FlutterError(
            code: "UNSUPPORTED",
            message: "DeviceCheck is not supported on this device",
            details: nil
          ))
        return
      }
      DCDevice.current.generateToken { token, error in
        if let error = error {
          result(
            FlutterError(
              code: "DEVICECHECK_FAILED",
              message: error.localizedDescription,
              details: nil
            ))
          return
        }
        guard let token = token else {
          result(
            FlutterError(code: "DEVICECHECK_FAILED", message: "no token", details: nil))
          return
        }
        result(token.base64EncodedString())
      }
    }
  }
}
