import 'dart:convert';

import 'package:http/http.dart' as http;

/// Typed API errors carrying the HTTP status so screens can render
/// accurate error states (404 empty vs 5xx retry).
class ApiError implements Exception {
  ApiError(this.status, this.message);
  final int status;
  final String message;
  @override
  String toString() => 'ApiError($status): $message';
}

class Post {
  Post.fromJson(Map<String, dynamic> j)
      : id = (j['id'] as num).toInt(),
        userId = (j['user_id'] as num).toInt(),
        author = (j['author'] as String?) ?? '',
        avatarUrl = (j['avatar_url'] as String?) ?? '',
        content = (j['content'] as String?) ?? '',
        township = (j['township'] as String?) ?? '',
        reactions = (j['reactions'] as num?)?.toInt() ?? 0,
        comments = (j['comments'] as num?)?.toInt() ?? 0;
  final int id, userId, reactions, comments;
  final String author, avatarUrl, content, township;
}

class RadarNode {
  RadarNode.fromJson(Map<String, dynamic> j)
      : nodeId = (j['node_id'] as String?) ?? '',
        handle = (j['handle'] as String?) ?? '',
        avatarUrl = (j['avatar_url'] as String?) ?? '',
        distance = (j['distance_bucket'] as String?) ?? '';
  final String nodeId, handle, avatarUrl, distance;
}

class Product {
  Product.fromJson(Map<String, dynamic> j)
      : id = (j['id'] as num).toInt(),
        title = (j['title'] as String?) ?? '',
        seller = (j['seller'] as String?) ?? '',
        priceCoins = (j['price_coins'] as num?)?.toInt() ?? 0,
        priceMmk = (j['price_mmk'] as num?)?.toInt() ?? 0,
        township = (j['township'] as String?) ?? '',
        imageUrl = (j['image_url'] as String?) ?? '';
  final int id, priceCoins, priceMmk;
  final String title, seller, township, imageUrl;
}

class ChatThread {
  ChatThread.fromJson(Map<String, dynamic> j)
      : id = (j['id'] as num).toInt(),
        mode = (j['mode'] as String?) ?? 'personal',
        peerId = (j['peer_id'] as num?)?.toInt() ?? 0,
        productTitle = (j['product_title'] as String?) ?? '';
  final int id, peerId;
  final String mode, productTitle;
}

class ChatMessage {
  ChatMessage.fromJson(Map<String, dynamic> j)
      : id = (j['id'] as num).toInt(),
        senderId = (j['sender_id'] as num).toInt(),
        body = (j['body'] as String?) ?? '';
  final int id, senderId;
  final String body;
}

class CoinTxn {
  CoinTxn.fromJson(Map<String, dynamic> j)
      : id = (j['id'] as num).toInt(),
        amount = (j['amount'] as num).toInt(),
        kind = (j['kind'] as String?) ?? '';
  final int id, amount;
  final String kind;
}

class DatingProfile {
  DatingProfile.fromJson(Map<String, dynamic> j)
      : nickname = (j['nickname'] as String?) ?? '',
        bio = (j['bio'] as String?) ?? '',
        hidden = (j['is_hidden_from_friends'] as bool?) ?? true;
  final String nickname, bio;
  final bool hidden;
}


class SocialStart {
  SocialStart.fromJson(Map<String, dynamic> j)
      : provider = (j['provider'] as String?) ?? '',
        authorizeUrl = (j['authorize_url'] as String?) ?? '',
        state = (j['state'] as String?) ?? '';
  final String provider, authorizeUrl, state;
}

class SocialLink {
  SocialLink.fromJson(Map<String, dynamic> j)
      : provider = (j['provider'] as String?) ?? '',
        providerUserId = (j['provider_user_id'] as String?) ?? '',
        linkedAt = (j['linked_at'] as String?) ?? '';
  final String provider, providerUserId, linkedAt;
}

/// NexusApi is the single REST client for the Flutter app. All business
/// rules live on the Go server; this client only serializes requests and
/// parses responses.
class NexusApi {
  NexusApi({http.Client? client, this.baseUrl = 'http://127.0.0.1:8080'})
      : _client = client ?? http.Client();
  final http.Client _client;
  final String baseUrl;
  String? accessToken;

  Map<String, String> _headers({bool auth = true}) => {
        'Content-Type': 'application/json',
        if (auth && accessToken != null)
          'Authorization': 'Bearer ${accessToken!}',
      };

  Future<Map<String, dynamic>> _send(String method, String path,
      {Object? body, bool auth = true}) async {
    final req = http.Request(method, Uri.parse('$baseUrl$path'));
    req.headers.addAll(_headers(auth: auth));
    if (body != null) {
      req.body = jsonEncode(body);
    }
    final streamed = await _client.send(req);
    final res = await http.Response.fromStream(streamed);
    Map<String, dynamic> json;
    try {
      json = res.body.isEmpty
          ? <String, dynamic>{}
          : jsonDecode(res.body) as Map<String, dynamic>;
    } catch (_) {
      throw ApiError(res.statusCode, 'invalid server response');
    }
    if (res.statusCode >= 400) {
      throw ApiError(
          res.statusCode, (json['error'] as String?) ?? 'HTTP ${res.statusCode}');
    }
    return json;
  }

  // ---- auth ----
  Future<Map<String, dynamic>> register(String phone, String displayName) =>
      _send('POST', '/api/v1/auth/register',
          body: {'phone': phone, 'display_name': displayName}, auth: false);
  Future<Map<String, dynamic>> verify(String phone, String code) =>
      _send('POST', '/api/v1/auth/verify',
          body: {'phone': phone, 'code': code}, auth: false);
  Future<Map<String, dynamic>> refresh(String refreshToken) =>
      _send('POST', '/api/v1/auth/refresh',
          body: {'refresh_token': refreshToken}, auth: false);

  // ---- device attestation (Play Integrity / DeviceCheck) ----
  Future<Map<String, dynamic>> attestNonce() =>
      _send('POST', '/api/v1/auth/attest/nonce');
  Future<Map<String, dynamic>> attestVerify(
          {required String nonce, required String platformToken}) =>
      _send('POST', '/api/v1/auth/attest/verify',
          body: {'nonce': nonce, 'platform_token': platformToken});

  // ---- profiles ----
  Future<Map<String, dynamic>> me() => _send('GET', '/api/v1/me');
  Future<Map<String, dynamic>> getProfile() => _send('GET', '/api/v1/profile');
  Future<Map<String, dynamic>> putProfile(
          String realName, String township, String avatarUrl) =>
      _send('PUT', '/api/v1/profile',
          body: {'real_name': realName, 'township': township, 'avatar_url': avatarUrl});
  Future<Map<String, dynamic>> getDatingProfile() =>
      _send('GET', '/api/v1/dating/profile');
  Future<Map<String, dynamic>> putDatingProfile(
          String nickname, String bio, bool hidden) =>
      _send('PUT', '/api/v1/dating/profile', body: {
        'nickname': nickname,
        'bio': bio,
        'is_hidden_from_friends': hidden
      });

  // ---- social ----
  Future<List<Post>> listPosts({String township = ''}) async {
    final q = township.isEmpty ? '' : 'township=${Uri.encodeQueryComponent(township)}&';
    final j = await _send('GET', '/api/v1/posts?${q}limit=20');
    return (j['posts'] as List)
        .map((e) => Post.fromJson(e as Map<String, dynamic>))
        .toList();
  }
  Future<Post> createPost(String content, String township) async {
    final j = await _send('POST', '/api/v1/posts',
        body: {'content': content, 'township': township});
    return Post.fromJson(j);
  }
  Future<Map<String, dynamic>> react(int postId, String kind) =>
      _send('POST', '/api/v1/posts/$postId/reactions', body: {'kind': kind});

  // ---- dating ----
  Future<Map<String, dynamic>> radar(double lat, double lng) =>
      _send('GET', '/api/v1/radar?lat=$lat&lng=$lng');
  Future<Map<String, dynamic>> swipe(String nodeId, String action) =>
      _send('POST', '/api/v1/dating/swipe', body: {'node_id': nodeId, 'action': action});
  Future<Map<String, dynamic>> whisperSend(String to, String body) =>
      _send('POST', '/api/v1/whisper', body: {'to': to, 'body': body});
  Future<Map<String, dynamic>> whisperRead(String id) =>
      _send('GET', '/api/v1/whisper/$id');

  // ---- coins ----
  Future<int> coinBalance() async {
    final j = await _send('GET', '/api/v1/coins/balance');
    return (j['balance'] as num).toInt();
  }
  Future<Map<String, dynamic>> topupDev(int amount, String requestId) =>
      _send('POST', '/api/v1/coins/topup/dev',
          body: {'amount': amount, 'request_id': requestId});
  Future<List<CoinTxn>> ledger() async {
    final j = await _send('GET', '/api/v1/coins/ledger');
    return (j['ledger'] as List)
        .map((e) => CoinTxn.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  // ---- market ----
  Future<List<Product>> products({String township = ''}) async {
    final q = township.isEmpty ? '' : 'township=${Uri.encodeQueryComponent(township)}&';
    final j = await _send('GET', '/api/v1/market/products?${q}limit=50');
    return (j['products'] as List)
        .map((e) => Product.fromJson(e as Map<String, dynamic>))
        .toList();
  }
  Future<Map<String, dynamic>> createProduct({
    required String title,
    required int priceCoins,
    required int priceMmk,
    required String township,
  }) =>
      _send('POST', '/api/v1/market/products', body: {
        'title': title,
        'price_coins': priceCoins,
        'price_mmk': priceMmk,
        'township': township,
      });
  Future<Map<String, dynamic>> order(int productId) =>
      _send('POST', '/api/v1/market/orders', body: {'product_id': productId});

  // ---- chat ----
  Future<ChatThread> createThread(String mode, {int? peerId, int? productId}) async {
    final j = await _send('POST', '/api/v1/chat/threads',
        body: {'mode': mode, 'peer_id': peerId ?? 0, 'product_id': productId ?? 0});
    return ChatThread.fromJson(j);
  }
  Future<List<ChatThread>> threads() async {
    final j = await _send('GET', '/api/v1/chat/threads');
    return (j['threads'] as List)
        .map((e) => ChatThread.fromJson(e as Map<String, dynamic>))
        .toList();
  }
  Future<List<ChatMessage>> messages(int threadId) async {
    final j = await _send('GET', '/api/v1/chat/threads/$threadId/messages');
    return (j['messages'] as List)
        .map((e) => ChatMessage.fromJson(e as Map<String, dynamic>))
        .toList();
  }
  Future<void> sendMessage(int threadId, String body) =>
      _send('POST', '/api/v1/chat/threads/$threadId/messages', body: {'body': body});
  // ---- social logins (Facebook / TikTok OAuth2, Telegram widget) ----
  Future<SocialStart> startSocial(String provider) async {
    final j = await _send('GET', '/api/v1/auth/social/$provider/start', auth: false);
    return SocialStart.fromJson(j);
  }

  Future<Map<String, dynamic>> socialCallback(
          String provider, String code, String state) =>
      _send(
          'GET',
          '/api/v1/auth/social/$provider/callback'
          '?code=${Uri.encodeQueryComponent(code)}'
          '&state=${Uri.encodeQueryComponent(state)}',
          auth: false);

  Future<Map<String, dynamic>> telegramLogin(Map<String, String> payload) =>
      _send('POST', '/api/v1/auth/social/telegram/callback',
          body: payload, auth: false);

  Future<List<SocialLink>> socialLinks() async {
    final j = await _send('GET', '/api/v1/auth/social/links');
    return (j['links'] as List)
        .map((e) => SocialLink.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<void> unlinkSocial(String provider) =>
      _send('DELETE', '/api/v1/auth/social/link/$provider');

  // ---- safety: 18+ gate, reports, blocks ----
  Future<Map<String, dynamic>> setBirthDate(String dateOfBirth) =>
      _send('PUT', '/api/v1/me/birthdate',
          body: {'date_of_birth': dateOfBirth});

  Future<Map<String, dynamic>> report({
    required String targetKind,
    required String targetId,
    required String reason,
    String details = '',
  }) =>
      _send('POST', '/api/v1/reports', body: {
        'target_kind': targetKind,
        'target_id': targetId,
        'reason': reason,
        'details': details,
      });

  Future<Map<String, dynamic>> block(String nodeId) =>
      _send('POST', '/api/v1/blocks', body: {'node_id': nodeId});

  Future<void> unblock(String nodeId) =>
      _send('DELETE', '/api/v1/blocks/${Uri.encodeComponent(nodeId)}');

  Future<List<Map<String, dynamic>>> blockedUsers() async {
    final j = await _send('GET', '/api/v1/blocks');
    return ((j['blocks'] as List?) ?? const [])
        .cast<Map<String, dynamic>>();
  }
}
