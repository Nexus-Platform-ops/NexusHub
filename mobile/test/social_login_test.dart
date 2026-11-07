import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:nexus_app/api.dart';
import 'package:nexus_app/social_login.dart';
import 'package:nexus_app/state.dart';

/// Test helper overriding the api provider.
Override apiProviderOverrideForTest(NexusApi api) =>
    apiProvider.overrideWithValue(api);

const _tokens = '{"access_token":"tok","refresh_token":"r","token_type":"Bearer",'
    '"user":{"id":7,"phone":"","display_name":"Nexus-1a2b3c4d","phone_verified":false}}';

http.Response _json(String body, int status) => http.Response(body, status,
    headers: {'content-type': 'application/json'});

NexusApi socialMockApi(Map<String, http.Response Function(http.Request)> routes) {
  return NexusApi(
    baseUrl: 'http://test',
    client: MockClient((req) async {
      final keys = routes.keys.toList()
        ..sort((a, b) => b.length.compareTo(a.length));
      for (final key in keys) {
        if (req.url.toString().contains(key)) {
          return routes[key]!(req);
        }
      }
      return _json('{"error":"unmocked ${req.url.path}"}', 404);
    }),
  );
}

void main() {
  test('startSocial parses authorize url and state', () async {
    final api = NexusApi(
        baseUrl: 'http://test',
        client: MockClient((req) async {
          expect(req.method, 'GET');
          expect(req.url.path, '/api/v1/auth/social/facebook/start');
          return _json(
              '{"provider":"facebook","authorize_url":"https://fb/oauth?state=s1","state":"s1"}',
              200);
        }));
    final s = await api.startSocial('facebook');
    expect(s.state, 's1');
    expect(s.authorizeUrl, contains('state=s1'));
  });

  test('socialCallback sends GET with encoded code and state', () async {
    String? url;
    final api = NexusApi(
        baseUrl: 'http://test',
        client: MockClient((req) async {
          url = req.url.toString();
          return _json(_tokens, 200);
        }));
    final res = await api.socialCallback('tiktok', 'c d', 's/t');
    expect(res['access_token'], 'tok');
    expect(url, contains('code=c+d'));
    expect(url, contains('state=s%2Ft'));
  });

  test('telegramLogin posts the widget payload', () async {
    String? method;
    String? path;
    Map<String, dynamic>? body;
    final api = NexusApi(
        baseUrl: 'http://test',
        client: MockClient((req) async {
          method = req.method;
          path = req.url.path;
          body = jsonDecode(req.body) as Map<String, dynamic>;
          return _json(_tokens, 200);
        }));
    await api.telegramLogin({'id': '1', 'hash': 'h'});
    expect(method, 'POST');
    expect(path, '/api/v1/auth/social/telegram/callback');
    expect(body!['id'], '1');
  });

  test('socialLinks parses providers without real identity fields', () async {
    final api = NexusApi(
        baseUrl: 'http://test',
        client: MockClient((req) async {
          expect(req.url.path, '/api/v1/auth/social/links');
          // The server intentionally returns only provider/id/time.
          return _json(
              '{"links":[{"provider":"facebook","provider_user_id":"fb-9",'
              '"linked_at":"2026-11-07T00:00:00Z"}]}',
              200);
        }));
    final links = await api.socialLinks();
    expect(links.single.provider, 'facebook');
    expect(links.single.providerUserId, 'fb-9');
  });

  test('unlinkSocial sends DELETE', () async {
    String? method;
    String? path;
    final api = NexusApi(
        baseUrl: 'http://test',
        client: MockClient((req) async {
          method = req.method;
          path = req.url.path;
          return _json('{"unlinked":true}', 200);
        }));
    await api.unlinkSocial('tiktok');
    expect(method, 'DELETE');
    expect(path, '/api/v1/auth/social/link/tiktok');
  });

  testWidgets('social login panel completes the facebook flow', (tester) async {
    final api = socialMockApi({
      '/api/v1/auth/social/facebook/start': (_) => _json(
          '{"provider":"facebook","authorize_url":"https://fb.example/oauth?state=st1","state":"st1"}',
          200),
      '/api/v1/auth/social/facebook/callback': (_) => _json(_tokens, 200),
    });
    await tester.pumpWidget(ProviderScope(
      overrides: [apiProviderOverrideForTest(api)],
      child: const MaterialApp(home: Scaffold(body: SocialLoginPanel())),
    ));
    await tester.tap(find.text('Continue with Facebook'));
    await tester.pumpAndSettle();
    expect(find.textContaining('fb.example'), findsOneWidget);
    await tester.enterText(
        find.byKey(const Key('social_code_field')), 'good-code');
    await tester.tap(find.text('Complete sign-in'));
    await tester.pumpAndSettle();
    expect(find.text('Signed in with facebook'), findsOneWidget);
    expect(api.accessToken, 'tok');
  });

  testWidgets('telegram sheet completes the widget login', (tester) async {
    final api = socialMockApi({
      '/api/v1/auth/social/telegram/callback': (_) => _json(_tokens, 200),
    });
    await tester.pumpWidget(ProviderScope(
      overrides: [apiProviderOverrideForTest(api)],
      child: const MaterialApp(home: Scaffold(body: SocialLoginPanel())),
    ));
    await tester.tap(find.text('Continue with Telegram'));
    await tester.pumpAndSettle();
    await tester.enterText(
        find.byKey(const Key('social_payload_field')),
        '{"id":"555","first_name":"Aung","auth_date":"4102444800","hash":"abc"}');
    await tester.tap(find.text('Complete sign-in'));
    await tester.pumpAndSettle();
    expect(find.text('Signed in with telegram'), findsOneWidget);
    expect(api.accessToken, 'tok');
  });

  testWidgets('provider error surfaces the error state', (tester) async {
    final api = socialMockApi({
      '/api/v1/auth/social/facebook/start': (_) =>
          _json('{"provider":"facebook","authorize_url":"https://fb/x","state":"s"}', 200),
      '/api/v1/auth/social/facebook/callback': (_) =>
          _json('{"error":"social provider not configured"}', 503),
    });
    await tester.pumpWidget(ProviderScope(
      overrides: [apiProviderOverrideForTest(api)],
      child: const MaterialApp(home: Scaffold(body: SocialLoginPanel())),
    ));
    await tester.tap(find.text('Continue with Facebook'));
    await tester.pumpAndSettle();
    await tester.enterText(
        find.byKey(const Key('social_code_field')), 'code');
    await tester.tap(find.text('Complete sign-in'));
    await tester.pumpAndSettle();
    expect(find.text('social provider not configured'), findsOneWidget);
  });
}
