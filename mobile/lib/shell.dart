import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'api.dart';
import 'paywall.dart';
import 'screens.dart';
import 'state.dart';
import 'theme.dart';

/// The five-tab shell replicating the prototype: Social, Dating, Market,
/// Chat, Profiles — with the hyper-local township selector and the Nexus
/// Coins badge in the sticky header.
class NexusShell extends ConsumerStatefulWidget {
  const NexusShell({super.key});
  @override
  ConsumerState<NexusShell> createState() => _NexusShellState();
}

class _NexusShellState extends ConsumerState<NexusShell> {
  int _index = 0;
  String _township = 'Kamayut';
  final ValueNotifier<int?> openThread = ValueNotifier<int?>(null);
  static const _townships = ['Kamayut', 'Bahan', 'Chanayethazan'];

  void _openChat(int threadId, String productTitle) {
    openThread.value = threadId;
    setState(() => _index = 3);
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(
      content: Text('Business chat opened for: $productTitle'),
    ));
  }

  void _showTopUp() {
    showModalBottomSheet<void>(
      context: context,
      shape: const RoundedRectangleBorder(
          borderRadius: BorderRadius.vertical(top: Radius.circular(24))),
      builder: (ctx) => SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(mainAxisSize: MainAxisSize.min, children: [
            const Row(children: [
              Icon(Icons.monetization_on, color: NexusColors.brand),
              SizedBox(width: 8),
              Text('Request Nexus Coins',
                  style: TextStyle(fontWeight: FontWeight.bold)),
            ]),
            const SizedBox(height: 12),
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceEvenly,
              children: [500, 1000, 2500].map(_topUpButton).toList(),
            ),
          ]),
        ),
      ),
    );
  }

  Widget _topUpButton(int amount) => OutlinedButton(
        style: OutlinedButton.styleFrom(
          foregroundColor: NexusColors.brand600,
          side: const BorderSide(color: NexusColors.brand),
        ),
        onPressed: () async {
          Navigator.of(context).pop();
          try {
            final res = await ref.read(apiProvider).topupDev(
                amount, 'ui-${DateTime.now().millisecondsSinceEpoch}');
            ref.invalidate(coinsProvider);
            if (!mounted) return;
            ScaffoldMessenger.of(context).showSnackBar(SnackBar(
                content: Text('Credited ${res['credited']} coins')));
          } on ApiError catch (e) {
            if (!mounted) return;
            ScaffoldMessenger.of(context)
                .showSnackBar(SnackBar(content: Text(e.message)));
          }
        },
        child: Text('+$amount'),
      );

  @override
  Widget build(BuildContext context) {
    final coins = ref.watch(coinsProvider);
    final isDark = Theme.of(context).brightness == Brightness.dark;
    return Scaffold(
      appBar: AppBar(
        title: Row(children: [
          Container(
            width: 36,
            height: 36,
            decoration: const BoxDecoration(
              gradient: LinearGradient(colors: [NexusColors.brand600, Color(0xFFFACC15)]),
              borderRadius: BorderRadius.all(Radius.circular(12)),
            ),
            child: const Icon(Icons.hive, color: Colors.white, size: 20),
          ),
          const SizedBox(width: 8),
          Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            ShaderMask(
              shaderCallback: (b) => const LinearGradient(
                colors: [NexusColors.brand600, Color(0xFFEAB308)],
              ).createShader(b),
              child: const Text('NexusHub',
                  style: TextStyle(
                      fontWeight: FontWeight.w800,
                      fontSize: 15,
                      color: Colors.white)),
            ),
            Text('MOBILE APP',
                style: TextStyle(
                    fontSize: 8,
                    letterSpacing: 2,
                    color: NexusColors.muted,
                    fontWeight: FontWeight.w600)),
          ]),
        ]),
        actions: [
          IconButton(
            icon: const Icon(Icons.star, color: NexusColors.brand),
            tooltip: 'Nexus+',
            onPressed: () {
              Navigator.of(context).push(
                MaterialPageRoute<void>(
                  builder: (_) => const ProviderScope(child: PaywallScreen()),
                ),
              );
            },
          ),
          PopupMenuButton<String>(
            initialValue: _township,
            onSelected: (v) {
              setState(() {
                _township = v;
                // Township filter applies to the feed and market.
                ref.invalidate(feedProvider(_township));
                ref.invalidate(marketProvider(_township));
              });
            },
            itemBuilder: (c) => _townships
                .map((t) => PopupMenuItem(value: t, child: Text(t)))
                .toList(),
            tooltip: 'Township Filter',
            child: Container(
              margin: const EdgeInsets.symmetric(horizontal: 4),
              padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
              decoration: BoxDecoration(
                color: isDark ? const Color(0xFF334155) : const Color(0xFFE2E8F0),
                borderRadius: BorderRadius.circular(999),
              ),
              child: Row(children: [
                const Icon(Icons.location_on, size: 14, color: NexusColors.brand),
                const SizedBox(width: 4),
                Text(_township,
                    style: const TextStyle(
                        fontSize: 12, fontWeight: FontWeight.w600)),
                const Icon(Icons.arrow_drop_down, size: 14, color: NexusColors.muted),
              ]),
            ),
          ),
          InkWell(
            onTap: _showTopUp,
            borderRadius: BorderRadius.circular(999),
            child: Container(
              margin: const EdgeInsets.only(right: 10),
              padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
              decoration: BoxDecoration(
                color: NexusColors.brand.withValues(alpha: 0.12),
                borderRadius: BorderRadius.circular(999),
                border: Border.all(color: NexusColors.brand.withValues(alpha: 0.4)),
              ),
              child: Row(children: [
                const Icon(Icons.monetization_on, size: 14, color: NexusColors.brand),
                const SizedBox(width: 4),
                coins.maybeWhen(
                  data: (v) => Text('$v',
                      style: const TextStyle(
                          fontSize: 12,
                          fontWeight: FontWeight.bold,
                          color: NexusColors.brand600)),
                  orElse: () => const Text('…',
                      style: TextStyle(fontSize: 12, color: NexusColors.brand600)),
                ),
              ]),
            ),
          ),
        ],
      ),
      body: IndexedStack(index: _index, children: [
        SocialScreen(township: _township),
        DatingScreen(township: _township),
        MarketScreen(township: _township, onChatOpened: _openChat),
        ChatScreen(openThread: openThread),
        ProfilesScreen(township: _township),
      ]),
      bottomNavigationBar: NavigationBar(
        selectedIndex: _index,
        onDestinationSelected: (i) => setState(() => _index = i),
        destinations: const [
          NavigationDestination(
              icon: Icon(Icons.article_outlined), selectedIcon: Icon(Icons.article),
              label: 'Social'),
          NavigationDestination(
              icon: Icon(Icons.favorite_outline, color: NexusColors.dating),
              selectedIcon: Icon(Icons.favorite, color: NexusColors.dating),
              label: 'Dating'),
          NavigationDestination(
              icon: Icon(Icons.storefront_outlined, color: NexusColors.market),
              selectedIcon: Icon(Icons.storefront, color: NexusColors.market),
              label: 'Market'),
          NavigationDestination(
              icon: Icon(Icons.forum_outlined, color: NexusColors.chat),
              selectedIcon: Icon(Icons.forum, color: NexusColors.chat),
              label: 'Chat'),
          NavigationDestination(
              icon: Icon(Icons.person_outline, color: NexusColors.profile),
              selectedIcon: Icon(Icons.person, color: NexusColors.profile),
              label: 'Profiles'),
        ],
      ),
    );
  }
}
