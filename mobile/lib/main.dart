import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:sentry_flutter/sentry_flutter.dart';

import 'login.dart';
import 'shell.dart';
import 'state.dart';
import 'theme.dart';

// Error reporting is opt-in: without a DSN the app runs exactly as before.
const _sentryDsn = String.fromEnvironment('SENTRY_DSN');
const _sentryEnvironment =
    String.fromEnvironment('SENTRY_ENVIRONMENT', defaultValue: 'development');
const _sentryRelease = String.fromEnvironment('SENTRY_RELEASE');

Future<void> main() async {
  if (_sentryDsn.isEmpty) {
    runApp(const ProviderScope(child: NexusApp()));
    return;
  }
  await SentryFlutter.init(
    (options) {
      options.dsn = _sentryDsn;
      options.environment = _sentryEnvironment;
      if (_sentryRelease.isNotEmpty) {
        options.release = _sentryRelease;
      }
      options.sendDefaultPii = false;
      options.tracesSampleRate = 0.0;
      options.attachStacktrace = true;
      options.attachScreenshot = false;
      // Never ship request URLs/query strings or user identifiers. The API
      // client sends auth only in headers, which are not collected here.
      options.beforeSend = (event, hint) {
        event.request = null;
        event.user = null;
        return event;
      };
    },
    appRunner: () => runApp(const ProviderScope(child: NexusApp())),
  );
}

class NexusApp extends StatelessWidget {
  const NexusApp({super.key});
  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'NexusHub',
      debugShowCheckedModeBanner: false,
      theme: nexusTheme(Brightness.light),
      darkTheme: nexusTheme(Brightness.dark),
      themeMode: ThemeMode.system,
      home: const AuthGate(),
    );
  }
}

/// Every screen in the shell calls authenticated endpoints, so the app must
/// not build it before a session exists.
class AuthGate extends ConsumerWidget {
  const AuthGate({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final verified = ref.watch(authProvider).verified;
    return verified ? const NexusShell() : const LoginScreen();
  }
}
