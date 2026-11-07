import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'state.dart';
import 'theme.dart';

/// Phone + OTP sign-in. The server stores the code in Redis and delivers it
/// through the configured SMS provider; when OTP_DEBUG=1 the register
/// response also carries the code so a local demo can finish without SMS.
class LoginScreen extends ConsumerStatefulWidget {
  const LoginScreen({super.key});

  @override
  ConsumerState<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends ConsumerState<LoginScreen> {
  final _phone = TextEditingController();
  final _name = TextEditingController();
  final _code = TextEditingController();
  String? _hint;

  static final _phoneRe = RegExp(r'^\+?[0-9]{7,15}$');

  @override
  void dispose() {
    _phone.dispose();
    _name.dispose();
    _code.dispose();
    super.dispose();
  }

  Future<void> _send() async {
    final phone = _phone.text.trim();
    if (!_phoneRe.hasMatch(phone)) {
      setState(() => _hint = 'Enter a phone number such as +959700000001');
      return;
    }
    var name = _name.text.trim();
    if (name.isNotEmpty && name.length < 2) {
      setState(() => _hint = 'Display name must be at least 2 characters');
      return;
    }
    if (name.isEmpty) name = 'Nexus user';
    setState(() => _hint = null);
    final ok = await ref.read(authProvider).register(phone, name);
    if (ok) _code.clear();
  }

  Future<void> _verify() async {
    final code = _code.text.trim();
    if (code.length < 4) {
      setState(() => _hint = 'Enter the verification code');
      return;
    }
    setState(() => _hint = null);
    await ref.read(authProvider).verify(code);
  }

  @override
  Widget build(BuildContext context) {
    final auth = ref.watch(authProvider);
    final error = _hint ?? auth.error;
    final awaitingCode = auth.pendingPhone != null;

    return Scaffold(
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Center(
                  child: Container(
                    width: 64,
                    height: 64,
                    decoration: BoxDecoration(
                      color: NexusColors.brand,
                      borderRadius: BorderRadius.circular(20),
                    ),
                    child: const Icon(Icons.hub, size: 34, color: Colors.white),
                  ),
                ),
                const SizedBox(height: 16),
                const Text('NexusHub',
                    textAlign: TextAlign.center,
                    style:
                        TextStyle(fontSize: 26, fontWeight: FontWeight.bold)),
                const SizedBox(height: 6),
                Text(
                  awaitingCode
                      ? 'Enter the code sent to ${auth.pendingPhone}'
                      : 'Sign in with your phone number',
                  textAlign: TextAlign.center,
                  style: TextStyle(
                      fontSize: 14,
                      color: Theme.of(context).colorScheme.onSurfaceVariant),
                ),
                const SizedBox(height: 24),
                Card(
                  child: Padding(
                    padding: const EdgeInsets.all(16),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        TextField(
                          key: const Key('phone-field'),
                          controller: _phone,
                          enabled: !auth.loading && !awaitingCode,
                          keyboardType: TextInputType.phone,
                          autofillHints: const [AutofillHints.telephoneNumber],
                          decoration: const InputDecoration(
                            labelText: 'Phone number',
                            hintText: '+959700000001',
                            prefixIcon: Icon(Icons.phone_outlined),
                          ),
                        ),
                        if (!awaitingCode) ...[
                          const SizedBox(height: 12),
                          TextField(
                            key: const Key('name-field'),
                            controller: _name,
                            enabled: !auth.loading,
                            maxLength: 80,
                            decoration: const InputDecoration(
                              labelText: 'Display name',
                              helperText: 'Optional, shown on your posts',
                              prefixIcon: Icon(Icons.person_outline),
                            ),
                          ),
                        ],
                        if (awaitingCode) ...[
                          const SizedBox(height: 12),
                          TextField(
                            key: const Key('otp-field'),
                            controller: _code,
                            enabled: !auth.loading,
                            keyboardType: TextInputType.number,
                            maxLength: 6,
                            autofocus: true,
                            decoration: const InputDecoration(
                              labelText: 'Verification code',
                              prefixIcon: Icon(Icons.lock_outline),
                            ),
                          ),
                          if (auth.devCode != null)
                            Padding(
                              padding: const EdgeInsets.only(bottom: 8),
                              child: Text(
                                'Development code: ${auth.devCode}',
                                key: const Key('dev-code'),
                                style: const TextStyle(
                                    fontSize: 12, color: NexusColors.brand700),
                              ),
                            ),
                        ],
                        if (error != null) ...[
                          const SizedBox(height: 4),
                          Text(error,
                              key: const Key('login-error'),
                              style: TextStyle(
                                  fontSize: 13,
                                  color: Theme.of(context).colorScheme.error)),
                        ],
                        const SizedBox(height: 16),
                        FilledButton(
                          key: Key(
                              awaitingCode ? 'verify-button' : 'send-button'),
                          onPressed: auth.loading
                              ? null
                              : (awaitingCode ? _verify : _send),
                          child: auth.loading
                              ? const SizedBox(
                                  width: 18,
                                  height: 18,
                                  child:
                                      CircularProgressIndicator(strokeWidth: 2))
                              : Text(awaitingCode
                                  ? 'Verify and continue'
                                  : 'Send code'),
                        ),
                        if (awaitingCode) ...[
                          const SizedBox(height: 4),
                          TextButton(
                            key: const Key('change-number'),
                            onPressed: auth.loading
                                ? null
                                : () {
                                    ref.read(authProvider).clearPending();
                                    setState(() {
                                      _hint = null;
                                      _code.clear();
                                    });
                                  },
                            child: const Text('Use a different number'),
                          ),
                        ],
                      ],
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
