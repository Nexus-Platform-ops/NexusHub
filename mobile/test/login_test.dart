import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:nexus_app/api.dart';
import 'package:nexus_app/main.dart';
import 'package:nexus_app/state.dart';

const _meJson =
    '{"id":7,"phone":"+959700000001","display_name":"Kyaw Zin","phone_verified":true,'
    '"real_name":"","township":"Kamayut","avatar_url":"","adult":true}';
const _tokensJson =
    '{"access_token":"tok","refresh_token":"r","token_type":"Bearer",'
    '"user":{"id":7,"phone":"+959700000001","display_name":"Kyaw Zin","phone_verified":true}}';

NexusApi loginApi() {
  final routes = <String, String>{
    '/api/v1/auth/register':
        '{"user_id":7,"phone":"+959700000001","otp_sent":true,"dev_code":"123456"}',
    '/api/v1/auth/verify': _tokensJson,
    '/api/v1/coins/balance': '{"balance":0}',
    '/api/v1/posts': '{"posts":[],"next_cursor":0}',
    '/api/v1/radar': '{"nodes":[],"degraded":false,"k_anonymity":3}',
    '/api/v1/market/products': '{"products":[]}',
    '/api/v1/chat/threads': '{"threads":[]}',
    '/api/v1/me': _meJson,
    '/api/v1/dating/profile': '{"error":"dating profile not created yet"}',
  };
  final keys = routes.keys.toList()
    ..sort((a, b) => b.length.compareTo(a.length));
  return NexusApi(
    baseUrl: 'http://test',
    client: MockClient((req) async {
      for (final key in keys) {
        if (req.url.toString().contains(key)) {
          return http.Response(routes[key]!, 200,
              headers: {'content-type': 'application/json'});
        }
      }
      return http.Response('{"error":"unmocked ${req.url.path}"}', 404,
          headers: {'content-type': 'application/json'});
    }),
  );
}

Future<void> pumpApp(WidgetTester tester, NexusApi api) async {
  await tester.pumpWidget(ProviderScope(
    overrides: [
      apiProvider.overrideWithValue(api),
      adultProvider.overrideWith((ref) async => true),
    ],
    child: const NexusApp(),
  ));
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('unauthenticated launch shows the login screen, not the shell',
      (tester) async {
    await pumpApp(tester, loginApi());

    expect(find.byKey(const Key('phone-field')), findsOneWidget);
    expect(find.byKey(const Key('send-button')), findsOneWidget);
    expect(find.text('Social'), findsNothing);
  });

  testWidgets('invalid phone is rejected before any request', (tester) async {
    await pumpApp(tester, loginApi());

    await tester.enterText(find.byKey(const Key('phone-field')), 'abc');
    await tester.tap(find.byKey(const Key('send-button')));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('otp-field')), findsNothing);
    expect(find.byKey(const Key('login-error')), findsOneWidget);
  });

  testWidgets('phone + OTP flow signs in and opens the shell', (tester) async {
    await pumpApp(tester, loginApi());

    await tester.enterText(
        find.byKey(const Key('phone-field')), '+959700000001');
    await tester.enterText(find.byKey(const Key('name-field')), 'Kyaw Zin');
    await tester.tap(find.byKey(const Key('send-button')));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('otp-field')), findsOneWidget);
    expect(find.text('Development code: 123456'), findsOneWidget);

    await tester.enterText(find.byKey(const Key('otp-field')), '123456');
    await tester.tap(find.byKey(const Key('verify-button')));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('phone-field')), findsNothing);
    expect(find.text('Social'), findsOneWidget);
    expect(find.text('Profiles'), findsOneWidget);
  });
}
