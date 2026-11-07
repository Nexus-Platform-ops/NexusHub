import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:nexus_app/api.dart';

void main() {
  group('model parsing', () {
    test('Post.fromJson parses all fields', () {
      final p = Post.fromJson({
        'id': 3,
        'user_id': 9,
        'author': 'A',
        'avatar_url': 'u',
        'content': 'hello',
        'township': 'Kamayut',
        'reactions': 12,
        'comments': 4,
      });
      expect(p.id, 3);
      expect(p.userId, 9);
      expect(p.author, 'A');
      expect(p.reactions, 12);
      expect(p.comments, 4);
    });

    test('RadarNode.fromJson parses anonymous handle and bucket', () {
      final n = RadarNode.fromJson({
        'node_id': 'abc',
        'handle': 'Bee-1F2E',
        'avatar_url': '',
        'distance_bucket': '1km',
      });
      expect(n.handle, 'Bee-1F2E');
      expect(n.distance, '1km');
    });

    test('Product.fromJson parses coin and mmk prices', () {
      final p = Product.fromJson({
        'id': 1,
        'title': 'Item',
        'seller': 'S',
        'price_coins': 120,
        'price_mmk': 15000,
        'township': 'Bahan',
        'image_url': '',
      });
      expect(p.priceCoins, 120);
      expect(p.priceMmk, 15000);
    });
  });

  group('NexusApi', () {
    NexusApi apiWith(http.Response Function(http.Request) fn) => NexusApi(
          baseUrl: 'http://test',
          client: MockClient((req) async => fn(req)),
        );

    test('listPosts parses posts array', () async {
      final api = apiWith((_) => http.Response(
          '{"posts":[{"id":1,"user_id":2,"author":"X","content":"c",'
          '"township":"K","reactions":1,"comments":0}],"next_cursor":0}',
          200));
      final posts = await api.listPosts();
      expect(posts, hasLength(1));
      expect(posts.first.content, 'c');
    });

    test('error responses raise ApiError with server message', () async {
      final api = apiWith(
          (_) => http.Response('{"error":"insufficient coins"}', 409));
      await expectLater(
        api.order(5),
        throwsA(isA<ApiError>()
            .having((e) => e.status, 'status', 409)
            .having((e) => e.message, 'message', 'insufficient coins')),
      );
    });

    test('non-JSON error bodies still raise ApiError', () async {
      final api =
          apiWith((_) => http.Response('Internal Server Error', 500));
      await expectLater(api.me(),
          throwsA(isA<ApiError>().having((e) => e.status, 'status', 500)));
    });

    test('coinBalance parses balance', () async {
      final api = apiWith((_) => http.Response('{"balance":2450}', 200));
      expect(await api.coinBalance(), 2450);
    });

    test('whisperRead surfaces 410 as ApiError', () async {
      final api =
          apiWith((_) => http.Response('{"error":"whisper expired"}', 410));
      await expectLater(api.whisperRead('x'),
          throwsA(isA<ApiError>().having((e) => e.status, 'status', 410)));
    });

    test('auth token is attached to authenticated requests', () async {
      late http.Request captured;
      final api = apiWith((req) {
        captured = req;
        return http.Response('{"balance":0}', 200);
      });
      api.accessToken = 'tok-123';
      await api.coinBalance();
      expect(captured.headers['Authorization'], 'Bearer tok-123');
    });
  });
}
