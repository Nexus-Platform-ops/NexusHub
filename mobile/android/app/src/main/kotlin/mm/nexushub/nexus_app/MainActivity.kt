package mm.nexushub.nexus_app

import com.google.android.play.core.integrity.IntegrityManagerFactory
import com.google.android.play.core.integrity.IntegrityTokenRequest
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel

class MainActivity : FlutterActivity() {
    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, CHANNEL)
            .setMethodCallHandler { call, result ->
                if (call.method != "getPlatformToken") {
                    result.notImplemented()
                    return@setMethodCallHandler
                }
                val nonce = call.argument<String>("nonce").orEmpty()
                if (nonce.isEmpty()) {
                    result.error("BAD_NONCE", "nonce is required", null)
                    return@setMethodCallHandler
                }
                val manager = IntegrityManagerFactory.create(applicationContext)
                val request = IntegrityTokenRequest.builder().setNonce(nonce).build()
                manager.requestIntegrityToken(request)
                    .addOnSuccessListener { response -> result.success(response.token()) }
                    .addOnFailureListener { e ->
                        result.error("INTEGRITY_FAILED", e.message, null)
                    }
            }
    }

    companion object {
        private const val CHANNEL = "nexus/attest"
    }
}
