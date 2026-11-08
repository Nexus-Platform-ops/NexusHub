import 'dart:async';

import 'package:flutter/services.dart';

import '../api.dart';

/// Device attestation client: nonce challenge → native platform token →
/// server verify. On Android the native side returns a Play Integrity token
/// bound to the nonce; on iOS a DeviceCheck token. Verification happens
/// server-side against Google/Apple; this class only shuttles the messages.
class AttestClient {
  AttestClient(this._api);

  final NexusApi _api;

  static const MethodChannel _channel = MethodChannel('nexus/attest');

  /// Full challenge flow. Returns true only when the server answered
  /// attested:true. Never throws: attestation is best-effort from the
  /// client's point of view — the server hard-gates discovery instead.
  Future<bool> attest() async {
    try {
      final nonceRes = await _api.attestNonce();
      final nonce = nonceRes['nonce'] as String?;
      if (nonce == null || nonce.isEmpty) return false;
      final token = await _channel.invokeMethod<String>(
        'getPlatformToken',
        <String, dynamic>{'nonce': nonce},
      );
      if (token == null || token.isEmpty) return false;
      final res = await _api.attestVerify(nonce: nonce, platformToken: token);
      return res['attested'] == true;
    } on PlatformException {
      return false;
    } on ApiError {
      return false;
    }
  }
}

/// Fire-and-forget attestation for the login flow: runs in the background,
/// swallows failures, never blocks navigation.
void attestInBackground(NexusApi api) {
  unawaited(AttestClient(api).attest());
}
