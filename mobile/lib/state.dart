import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'api.dart';

/// Base URL override for tests and device builds.
const String kDefaultApiBaseUrl = String.fromEnvironment(
  'NEXUS_API_URL',
  defaultValue: 'http://10.0.2.2:8080',
);

final apiProvider =
    Provider<NexusApi>((ref) => NexusApi(baseUrl: kDefaultApiBaseUrl));

/// Auth controller drives the phone + OTP flow. In development builds the
/// server (OTP_DEBUG=1) returns the OTP in the register response so the
/// demo can complete the flow end to end.
class AuthController extends ChangeNotifier {
  AuthController(this._ref);
  final Ref _ref;

  bool loading = false;
  String? error;
  String? pendingPhone;
  int? userId;
  bool verified = false;

  /// Code echoed by the server when OTP_DEBUG=1; local builds show it so the
  /// flow can be completed without an SMS provider.
  String? devCode;

  NexusApi get _api => _ref.read(apiProvider);

  Future<bool> register(String phone, String displayName) async {
    loading = true;
    error = null;
    notifyListeners();
    try {
      final res = await _api.register(phone, displayName);
      pendingPhone = phone;
      userId = (res['user_id'] as num).toInt();
      devCode = res['dev_code'] as String?;
      return true;
    } on ApiError catch (e) {
      error = e.message;
      return false;
    } finally {
      loading = false;
      notifyListeners();
    }
  }

  Future<bool> verify(String code) async {
    if (pendingPhone == null) return false;
    loading = true;
    error = null;
    notifyListeners();
    try {
      final res = await _api.verify(pendingPhone!, code);
      _api.accessToken = res['access_token'] as String?;
      userId = ((res['user'] as Map<String, dynamic>)['id'] as num).toInt();
      verified = true;
      devCode = null;
      return true;
    } on ApiError catch (e) {
      error = e.message;
      return false;
    } finally {
      loading = false;
      notifyListeners();
    }
  }

  /// Completes a social login (Facebook / TikTok / Telegram): the server
  /// has already verified the provider payload, the response carries the
  /// standard token pair and the pseudonymous account.
  void completeSocialLogin(Map<String, dynamic> res) {
    _api.accessToken = res['access_token'] as String?;
    final user = res['user'] as Map<String, dynamic>?;
    userId = user == null ? null : ((user['id'] as num).toInt());
    verified = true;
    pendingPhone = null;
    devCode = null;
    // coinsProvider watches authProvider, so notifyListeners() alone
    // refreshes it — invalidating explicitly here would create a cycle.
    notifyListeners();
  }

  void logout() {
    _api.accessToken = null;
    verified = false;
    userId = null;
    pendingPhone = null;
    devCode = null;
    notifyListeners();
  }

  /// Abandons a pending OTP challenge so the user can correct the number.
  void clearPending() {
    pendingPhone = null;
    devCode = null;
    error = null;
    notifyListeners();
  }
}

final authProvider =
    ChangeNotifierProvider<AuthController>((ref) => AuthController(ref));

/// Coin balance, refreshed after top-ups and purchases.
final coinsProvider = FutureProvider<int>((ref) async {
  final auth = ref.watch(authProvider);
  if (!auth.verified) return 0;
  return ref.read(apiProvider).coinBalance();
});

/// Social feed, keyed by township filter.
final feedProvider =
    FutureProvider.autoDispose.family<List<Post>, String>((ref, township) {
  return ref.read(apiProvider).listPosts(township: township);
});

/// Hyper-local market listings, keyed by township filter.
final marketProvider =
    FutureProvider.autoDispose.family<List<Product>, String>((ref, township) {
  return ref.read(apiProvider).products(township: township);
});

/// Anonymous radar nodes for the demo location (Kamayut, Yangon).
final radarProvider = FutureProvider.autoDispose<Map<String, dynamic>>((ref) {
  return ref.read(apiProvider).radar(16.8, 96.15);
});

/// Unified chat threads.
final threadsProvider = FutureProvider.autoDispose<List<ChatThread>>((ref) {
  return ref.read(apiProvider).threads();
});

/// Current user + user profile (main account).
final profileProvider = FutureProvider.autoDispose<Map<String, dynamic>>((ref) {
  return ref.read(apiProvider).me();
});

/// Whether the signed-in account has verified an 18+ birth date. Dating,
/// Radar and Whisper are gated on this server-side as well.
final adultProvider = FutureProvider.autoDispose<bool>((ref) async {
  final auth = ref.watch(authProvider);
  if (!auth.verified) return false;
  final me = await ref.read(apiProvider).me();
  return me['adult'] == true;
});

/// Dating profile (isolated schema).
final datingProfileProvider =
    FutureProvider.autoDispose<Map<String, dynamic>>((ref) {
  return ref.read(apiProvider).getDatingProfile();
});
