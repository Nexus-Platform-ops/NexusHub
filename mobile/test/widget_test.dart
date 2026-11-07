import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:nexus_app/api.dart';
import 'package:nexus_app/main.dart';
import 'package:nexus_app/state.dart';

const postsJson = '{"posts":[{"id":1,"user_id":11,"author":"Kyaw Zin Aung",'
    '"avatar_url":"","content":"ကမာရွတ် coffee shop ☕","image_url":"",'
    '"township":"Kamayut","created_at":"2026-11-07T03:00:00Z","reactions":142,'
    '"comments":28}],"next_cursor":0}';
const radarJson =
    '{"nodes":[{"node_id":"ZCQ","handle":"Bee-A1B2","avatar_url":"",'
    '"distance_bucket":"500m"}],"degraded":false,"k_anonymity":3}';
const radarDegraded = '{"nodes":[],"degraded":true,"k_anonymity":3}';
const productsJson =
    '{"products":[{"id":5,"seller_id":2,"seller":"Seller","title":"Sony Headphones",'
    '"description":"","price_coins":350,"price_mmk":0,"township":"Kamayut",'
    '"image_url":"","active":true,"created_at":"2026-11-07T03:00:00Z"}]}';
const threadsJson = '{"threads":[]}';
const meJson =
    '{"id":11,"phone":"+959700000001","display_name":"Thandar Myint",'
    '"phone_verified":true,"real_name":"Thandar Myint","township":"Kamayut",'
    '"avatar_url":"","adult":true}';
const balanceJson = '{"balance":2450}';

/// Builds a fully mocked NexusApi whose responses are keyed by URL fragment.
NexusApi mockApi(Map<String, String Function(http.Request)> routes) {
  return NexusApi(
    baseUrl: 'http://test',
    client: MockClient((req) async {
      final keys = routes.keys.toList()
        ..sort((a, b) => b.length.compareTo(a.length));
      for (final key in keys) {
        if (req.url.toString().contains(key)) {
          return http.Response(routes[key]!(req), 200,
              headers: {'content-type': 'application/json'});
        }
      }
      return http.Response('{"error":"unmocked ${req.url.path}"}', 404);
    }),
  );
}

Map<String, String Function(http.Request)> defaultRoutes() => {
      '/api/v1/coins/balance': (_) => balanceJson,
      '/api/v1/posts': (_) => postsJson,
      '/api/v1/radar': (_) => radarJson,
      '/api/v1/market/products': (_) => productsJson,
      '/api/v1/chat/threads': (_) => threadsJson,
      '/api/v1/me': (_) => meJson,
      '/api/v1/dating/profile': (_) =>
          '{"error":"dating profile not created yet"}',
    };

Future<void> pumpApp(WidgetTester tester, NexusApi api,
    {bool adult = true}) async {
  await tester.pumpWidget(ProviderScope(
    overrides: [
      apiProvider.overrideWithValue(api),
      authProvider.overrideWith((ref) {
        final auth = AuthController(ref);
        auth.verified = true;
        auth.userId = 11;
        return auth;
      }),
      adultProvider.overrideWith((ref) async => adult),
    ],
    child: const NexusApp(),
  ));
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('social feed renders posts from the API', (tester) async {
    await pumpApp(tester, mockApi(defaultRoutes()));
    expect(find.text('Kyaw Zin Aung'), findsOneWidget);
    expect(find.text('Social'), findsOneWidget);
    expect(find.text('Dating'), findsOneWidget);
    expect(find.text('Market'), findsOneWidget);
    expect(find.text('Chat'), findsOneWidget);
    expect(find.text('Profiles'), findsOneWidget);
  });

  testWidgets('like button toggles via API and refreshes feed', (tester) async {
    var reacted = false;
    final routes = defaultRoutes();
    routes['/api/v1/posts/1/reactions'] = (_) {
      reacted = true;
      return '{"liked":true,"reactions":143}';
    };
    await pumpApp(tester, mockApi(routes));
    await tester.tap(find.text('Like').first);
    await tester.pumpAndSettle();
    expect(reacted, isTrue);
  });

  testWidgets('bottom navigation switches to market and dating tabs',
      (tester) async {
    await pumpApp(tester, mockApi(defaultRoutes()));
    await tester.tap(find.text('Market'));
    await tester.pumpAndSettle();
    expect(find.text('Hyper-Local Marketplace'), findsOneWidget);
    expect(find.text('Sony Headphones'), findsOneWidget);

    await tester.tap(find.text('Dating'));
    await tester.pumpAndSettle();
    expect(find.textContaining('Bee-A1B2'), findsOneWidget);
  });

  testWidgets('empty feed shows the empty state', (tester) async {
    final routes = defaultRoutes();
    routes['/api/v1/posts'] = (_) => '{"posts":[],"next_cursor":0}';
    await pumpApp(tester, mockApi(routes));
    expect(find.byIcon(Icons.article_outlined), findsOneWidget);
    expect(find.text('No posts in Kamayut yet — be the first!'), findsOneWidget);
  });

  testWidgets('API failure shows the error state with retry', (tester) async {
    final routes = defaultRoutes();
    routes['/api/v1/posts'] = (_) => '{"error":"database down"}';
    await pumpApp(tester, mockApi(routes));
    expect(find.byIcon(Icons.cloud_off), findsOneWidget);
    expect(find.text('Retry'), findsOneWidget);
  });

  testWidgets('dating tab is gated to 18+ adults', (tester) async {
    await pumpApp(tester, mockApi(defaultRoutes()), adult: false);
    await tester.tap(find.text('Dating'));
    await tester.pumpAndSettle();
    expect(find.text('Adults only (18+)'), findsOneWidget);
    expect(find.textContaining('Bee-A1B2'), findsNothing);
  });

  testWidgets('degraded radar explains k-anonymity protection', (tester) async {
    final routes = defaultRoutes();
    routes['/api/v1/radar'] = (_) => radarDegraded;
    await pumpApp(tester, mockApi(routes));
    await tester.tap(find.text('Dating'));
    await tester.pumpAndSettle();
    expect(find.textContaining('protect member privacy'), findsOneWidget);
  });
}
