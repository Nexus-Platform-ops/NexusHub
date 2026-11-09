import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'api.dart';
import 'dating_ui.dart';
import 'state.dart';
import 'social_login.dart';
import 'theme.dart';
import 'widgets.dart';

// ---------------------------------------------------------------------------
// VIEW 1 — Social feed (Facebook-style)
// ---------------------------------------------------------------------------
class SocialScreen extends ConsumerStatefulWidget {
  const SocialScreen({super.key, required this.township});
  final String township;
  @override
  ConsumerState<SocialScreen> createState() => _SocialScreenState();
}

class _SocialScreenState extends ConsumerState<SocialScreen> {
  final _postController = TextEditingController();

  @override
  void dispose() {
    _postController.dispose();
    super.dispose();
  }

  void _toast(String message) {
    if (!mounted) return;
    ScaffoldMessenger.of(context)
        .showSnackBar(SnackBar(content: Text(message)));
  }

  @override
  Widget build(BuildContext context) {
    final feed = ref.watch(feedProvider(widget.township));
    final me = ref.watch(profileProvider);
    final name = me.value?['display_name'] as String? ?? '';
    return Column(
      children: [
        Card(
          margin: const EdgeInsets.fromLTRB(12, 12, 12, 0),
          child: Padding(
            padding: const EdgeInsets.all(12),
            child: Row(children: [
              CircleAvatar(
                radius: 18,
                backgroundColor: NexusColors.brand.withValues(alpha: 0.2),
                child: Text(name.isNotEmpty ? name[0] : 'N',
                    style: const TextStyle(
                        color: NexusColors.brand700,
                        fontWeight: FontWeight.bold)),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: TextField(
                  controller: _postController,
                  decoration:
                      const InputDecoration(hintText: "What's on your mind?"),
                ),
              ),
              const SizedBox(width: 8),
              FilledButton(
                onPressed: () async {
                  final text = _postController.text.trim();
                  if (text.isEmpty) return;
                  try {
                    await ref
                        .read(apiProvider)
                        .createPost(text, widget.township);
                    if (!mounted) return;
                    _postController.clear();
                    FocusManager.instance.primaryFocus?.unfocus();
                    ref.invalidate(feedProvider(widget.township));
                  } on ApiError catch (e) {
                    _toast(e.message);
                  }
                },
                child: const Text('Post'),
              ),
            ]),
          ),
        ),
        Expanded(
          child: StateSwitch(
            value: feed,
            emptyIcon: Icons.article_outlined,
            emptyLabel: 'No posts in ${widget.township} yet — be the first!',
            onData: (posts) => RefreshIndicator(
              onRefresh: () async =>
                  ref.refresh(feedProvider(widget.township).future),
              child: ListView.builder(
                padding: const EdgeInsets.all(12),
                itemCount: posts.length,
                itemBuilder: (c, i) => PostCard(
                  post: posts[i],
                  onLike: () async {
                    try {
                      await ref.read(apiProvider).react(posts[i].id, 'like');
                      ref.invalidate(feedProvider(widget.township));
                    } on ApiError catch (e) {
                      _toast(e.message);
                    }
                  },
                ),
              ),
            ),
          ),
        ),
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// VIEW 2 — Dating: Radar / Cards / Whisper sub-tabs (rose accents)
// ---------------------------------------------------------------------------
class DatingScreen extends ConsumerStatefulWidget {
  const DatingScreen({super.key, required this.township});
  final String township;
  @override
  ConsumerState<DatingScreen> createState() => _DatingScreenState();
}

class _DatingScreenState extends ConsumerState<DatingScreen> {
  int _tab = 0; // 0 radar, 1 cards, 2 whisper

  @override
  Widget build(BuildContext context) {
    final adult = ref.watch(adultProvider);
    return adult.when(
      loading: () => const LoadingView(label: 'Checking age settings…'),
      error: (e, _) => ErrorView(
          message: e.toString(), onRetry: () => ref.invalidate(adultProvider)),
      data: (ok) => ok ? _datingBody() : const _AgeGateView(),
    );
  }

  Widget _datingBody() {
    return Column(children: [
      Padding(
        padding: const EdgeInsets.fromLTRB(12, 12, 12, 0),
        child: SegmentedButton<int>(
          segments: const [
            ButtonSegment(value: 0, label: Text('Radar')),
            ButtonSegment(value: 1, label: Text('Cards')),
            ButtonSegment(value: 2, label: Text('Whisper')),
          ],
          selected: {_tab},
          selectedIcon: const Icon(Icons.favorite, color: Colors.white),
          onSelectionChanged: (s) => setState(() => _tab = s.first),
          style: ButtonStyle(
            side: WidgetStatePropertyAll(
                BorderSide(color: NexusColors.dating.withValues(alpha: 0.4))),
          ),
        ),
      ),
      Expanded(
        child: switch (_tab) {
          0 => const _RadarView(),
          1 => _CardsView(onWhisper: () => setState(() => _tab = 2)),
          _ => const _WhisperView(),
        },
      ),
    ]);
  }
}

class _RadarView extends ConsumerWidget {
  const _RadarView();
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final radar = ref.watch(radarProvider);
    return radar.when(
      loading: () => const LoadingView(label: 'Scanning BeeTalk Radar…'),
      error: (e, _) => ErrorView(message: e.toString(), onRetry: () => ref.invalidate(radarProvider)),
      data: (d) {
        final nodes = ((d['nodes'] as List?) ?? const [])
            .cast<Map<String, dynamic>>();
        return AnimatedRadar(nodes: nodes);
      },
    );
  }
}

class _CardsView extends ConsumerStatefulWidget {
  const _CardsView({required this.onWhisper});
  final VoidCallback onWhisper;
  @override
  ConsumerState<_CardsView> createState() => _CardsViewState();
}

class _CardsViewState extends ConsumerState<_CardsView> {
  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  @override
  Widget build(BuildContext context) {
    final radar = ref.watch(radarProvider);
    return radar.when(
      loading: () => const LoadingView(),
      error: (e, _) => ErrorView(message: e.toString(), onRetry: () => ref.invalidate(radarProvider)),
      data: (d) {
        final nodes = ((d['nodes'] as List?) ?? const [])
            .cast<Map<String, dynamic>>();
        return SwipeableCardStack(
          nodes: nodes,
          onSwipe: (nodeId, action) async {
            try {
              final res = await ref.read(apiProvider).swipe(nodeId, action);
              if (res['matched'] == true) {
                widget.onWhisper();
                return true;
              }
              return false;
            } on ApiError catch (e) {
              _toast(e.message);
              return false;
            }
          },
        );
      },
    );
  }
}

class _WhisperView extends ConsumerStatefulWidget {
  const _WhisperView();
  @override
  ConsumerState<_WhisperView> createState() => _WhisperViewState();
}

class _WhisperViewState extends ConsumerState<_WhisperView> {
  final _controller = TextEditingController();
  final List<String> _bubbles = [];
  final List<Timer> _timers = [];

  @override
  void dispose() {
    for (final t in _timers) {
      t.cancel();
    }
    _controller.dispose();
    super.dispose();
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  @override
  Widget build(BuildContext context) {
    final radar = ref.watch(radarProvider);
    final nodes = (radar.value?['nodes'] as List?) ?? const [];
    final target =
        nodes.isEmpty ? null : nodes.first as Map<String, dynamic>;
    return Padding(
      padding: const EdgeInsets.all(12),
      child: Column(children: [
        Row(children: [
          const Icon(Icons.admin_panel_settings,
              size: 16, color: NexusColors.whisper),
          const SizedBox(width: 6),
          const Expanded(
            child: Text('BeeTalk Whisper Chat',
                style: TextStyle(fontWeight: FontWeight.bold, fontSize: 12)),
          ),
          if (target != null)
            PopupMenuButton<String>(
              tooltip: 'Safety actions',
              icon: const Icon(Icons.more_vert, size: 18),
              onSelected: (action) {
                if (action == 'report') {
                  _showReportSheet(context, ref,
                      targetKind: 'user',
                      targetId: target['node_id'] as String,
                      label: '${target['handle']}');
                } else {
                  _confirmBlock(context, ref,
                      nodeId: target['node_id'] as String,
                      label: '${target['handle']}');
                }
              },
              itemBuilder: (_) => const [
                PopupMenuItem(value: 'report', child: Text('Report')),
                PopupMenuItem(value: 'block', child: Text('Block')),
              ],
            ),
        ]),
        const Align(
          alignment: Alignment.centerLeft,
          child: Text('Messages auto-delete immediately after read (5s TTL).',
              style: TextStyle(fontSize: 10, color: NexusColors.muted)),
        ),
        const SizedBox(height: 8),
        Expanded(
          child: Container(
            width: double.infinity,
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: Theme.of(context).brightness == Brightness.dark
                  ? NexusColors.bgDark
                  : const Color(0xFFF8FAFC),
              borderRadius: BorderRadius.circular(12),
            ),
            child: _bubbles.isEmpty
                ? const Center(
                    child: Text('Whisper chat active — say something 👋',
                        style: TextStyle(
                            fontSize: 10,
                            fontStyle: FontStyle.italic,
                            color: NexusColors.muted)))
                : ListView.builder(
                    itemCount: _bubbles.length,
                    itemBuilder: (c, i) => Align(
                      alignment: Alignment.centerRight,
                      child: Container(
                        margin: const EdgeInsets.only(bottom: 6),
                        padding: const EdgeInsets.all(8),
                        constraints: const BoxConstraints(maxWidth: 260),
                        decoration: BoxDecoration(
                          color: NexusColors.whisper,
                          borderRadius: BorderRadius.circular(16),
                        ),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.end,
                          children: [
                            Text(_bubbles[i],
                                style: const TextStyle(
                                    color: Colors.white, fontSize: 12)),
                            const Text('Self-destructs in 5s…',
                                style: TextStyle(
                                    color: Colors.white70,
                                    fontSize: 8,
                                    fontStyle: FontStyle.italic)),
                          ],
                        ),
                      ),
                    ),
                  ),
          ),
        ),
        const SizedBox(height: 8),
        Row(children: [
          Expanded(
            child: TextField(
              controller: _controller,
              decoration:
                  const InputDecoration(hintText: 'Send self-destruct whisper…'),
            ),
          ),
          const SizedBox(width: 8),
          IconButton.filled(
            key: const Key('whisper-send'),
            style: IconButton.styleFrom(backgroundColor: NexusColors.whisper),
            onPressed: nodes.isEmpty
                ? null
                : () async {
                    final text = _controller.text.trim();
                    if (text.isEmpty) return;
                    final node = nodes.first as Map<String, dynamic>;
                    try {
                      final res = await ref
                          .read(apiProvider)
                          .whisperSend(node['node_id'] as String, text);
                      _controller.clear();
                      setState(() => _bubbles.add(text));
                      _timers.add(Timer(const Duration(seconds: 5), () {
                        if (mounted) setState(() => _bubbles.remove(text));
                      }));
                      // Demonstrate read-once semantics (message is now gone).
                      await ref
                          .read(apiProvider)
                          .whisperRead(res['id'] as String);
                    } on ApiError catch (e) {
                      _toast(e.message);
                    }
                  },
            icon: const Icon(Icons.send, color: Colors.white),
          ),
        ]),
      ]),
    );
  }
}

// 18+ gate shown before Radar / Cards / Whisper are usable. The server
// enforces the same rule on every dating endpoint.
class _AgeGateView extends ConsumerStatefulWidget {
  const _AgeGateView();
  @override
  ConsumerState<_AgeGateView> createState() => _AgeGateViewState();
}

class _AgeGateViewState extends ConsumerState<_AgeGateView> {
  bool _saving = false;

  static String _iso(DateTime d) => '${d.year.toString().padLeft(4, '0')}-'
      '${d.month.toString().padLeft(2, '0')}-'
      '${d.day.toString().padLeft(2, '0')}';

  Future<void> _pick() async {
    final now = DateTime.now();
    final picked = await showDatePicker(
      context: context,
      initialDate: DateTime(now.year - 20, now.month, now.day),
      firstDate: DateTime(1900),
      lastDate: now,
      helpText: 'Select your date of birth',
    );
    if (picked == null || !mounted) return;
    setState(() => _saving = true);
    try {
      final res = await ref.read(apiProvider).setBirthDate(_iso(picked));
      if (!mounted) return;
      if (res['adult'] == true) {
        ref.invalidate(adultProvider);
        ref.invalidate(radarProvider);
        ref.invalidate(datingProfileProvider);
      } else {
        setState(() => _saving = false);
        ScaffoldMessenger.of(context).showSnackBar(const SnackBar(
            content: Text(
                'Dating and Radar are only available to users aged 18 or over.')));
      }
    } on ApiError catch (e) {
      if (!mounted) return;
      setState(() => _saving = false);
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  @override
  Widget build(BuildContext context) {
    return Center(
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(24),
        child: Column(mainAxisAlignment: MainAxisAlignment.center, children: [
          const Icon(Icons.shield_outlined, size: 64, color: NexusColors.dating),
          const SizedBox(height: 12),
          const Text('Adults only (18+)',
              style: TextStyle(fontSize: 18, fontWeight: FontWeight.bold)),
          const SizedBox(height: 8),
          Text(
            'Dating, anonymous Radar and self-destruct Whisper are restricted '
            'to users aged 18 and over. Your birth date stays on your main '
            'account and is never shown in your dating profile.',
            textAlign: TextAlign.center,
            style: TextStyle(fontSize: 11, color: NexusColors.muted),
          ),
          const SizedBox(height: 20),
          FilledButton.icon(
            style: FilledButton.styleFrom(backgroundColor: NexusColors.dating),
            onPressed: _saving ? null : _pick,
            icon: _saving
                ? const SizedBox(
                    width: 16,
                    height: 16,
                    child: CircularProgressIndicator(
                        strokeWidth: 2, color: Colors.white))
                : const Icon(Icons.cake_outlined),
            label: Text(_saving ? 'Checking…' : 'Confirm my date of birth'),
          ),
        ]),
      ),
    );
  }
}

// Long-press a radar node for safety actions.
Future<void> _showNodeActions(
    BuildContext context, WidgetRef ref, Map<String, dynamic> node) async {
  final nodeId = node['node_id'] as String? ?? '';
  if (nodeId.isEmpty) return;
  final label = node['handle'] as String? ?? 'this account';
  final action = await showModalBottomSheet<String>(
    context: context,
    builder: (ctx) => SafeArea(
      child: Column(mainAxisSize: MainAxisSize.min, children: [
        ListTile(
          leading: const Icon(Icons.flag_outlined),
          title: const Text('Report'),
          onTap: () => Navigator.pop(ctx, 'report'),
        ),
        ListTile(
          leading: const Icon(Icons.block),
          title: const Text('Block'),
          onTap: () => Navigator.pop(ctx, 'block'),
        ),
      ]),
    ),
  );
  if (!context.mounted) return;
  if (action == 'report') {
    await _showReportSheet(context, ref,
        targetKind: 'user', targetId: nodeId, label: label);
  } else if (action == 'block') {
    await _confirmBlock(context, ref, nodeId: nodeId, label: label);
  }
}

Future<void> _showReportSheet(
  BuildContext context,
  WidgetRef ref, {
  required String targetKind,
  required String targetId,
  String label = '',
}) async {
  const reasons = {
    'harassment': 'Harassment or threats',
    'spam': 'Spam',
    'scam': 'Scam or fraud',
    'nudity': 'Nudity or sexual content',
    'underage': 'Underage user',
    'impersonation': 'Impersonation',
    'other': 'Other',
  };
  var reason = 'harassment';
  final details = TextEditingController();
  final submitted = await showModalBottomSheet<bool>(
    context: context,
    isScrollControlled: true,
    builder: (ctx) => StatefulBuilder(
      builder: (ctx, setSheet) => Padding(
        padding: EdgeInsets.fromLTRB(
            16, 16, 16, MediaQuery.of(ctx).viewInsets.bottom + 16),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Report ${label.isEmpty ? 'this account' : label}',
                style: const TextStyle(fontWeight: FontWeight.bold)),
            const SizedBox(height: 8),
            DropdownButtonFormField<String>(
              initialValue: reason,
              items: [
                for (final e in reasons.entries)
                  DropdownMenuItem(value: e.key, child: Text(e.value)),
              ],
              onChanged: (v) => setSheet(() => reason = v ?? reason),
              decoration: const InputDecoration(labelText: 'Reason'),
            ),
            const SizedBox(height: 8),
            TextField(
              controller: details,
              maxLength: 1000,
              maxLines: 3,
              decoration: const InputDecoration(
                  hintText: 'What happened? (optional)'),
            ),
            const SizedBox(height: 4),
            SizedBox(
              width: double.infinity,
              child: FilledButton(
                onPressed: () async {
                  try {
                    await ref.read(apiProvider).report(
                          targetKind: targetKind,
                          targetId: targetId,
                          reason: reason,
                          details: details.text.trim(),
                        );
                    if (ctx.mounted) Navigator.of(ctx).pop(true);
                  } on ApiError catch (e) {
                    if (ctx.mounted) {
                      ScaffoldMessenger.of(ctx).showSnackBar(
                          SnackBar(content: Text(e.message)));
                    }
                  }
                },
                child: const Text('Submit report'),
              ),
            ),
          ],
        ),
      ),
    ),
  );
  details.dispose();
  if (submitted == true && context.mounted) {
    ScaffoldMessenger.of(context).showSnackBar(const SnackBar(
        content:
            Text('Report submitted. Our safety team will review it.')));
  }
}

Future<void> _confirmBlock(
  BuildContext context,
  WidgetRef ref, {
  required String nodeId,
  required String label,
}) async {
  final ok = await showDialog<bool>(
    context: context,
    builder: (ctx) => AlertDialog(
      title: Text('Block $label?'),
      content: const Text(
          'They will disappear from Radar and can no longer whisper you. '
          'You can unblock them later.'),
      actions: [
        TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('Cancel')),
        FilledButton(
          style: FilledButton.styleFrom(backgroundColor: NexusColors.dating),
          onPressed: () => Navigator.pop(ctx, true),
          child: const Text('Block'),
        ),
      ],
    ),
  );
  if (ok != true || !context.mounted) return;
  try {
    await ref.read(apiProvider).block(nodeId);
    ref.invalidate(radarProvider);
    if (context.mounted) {
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text('Blocked $label.')));
    }
  } on ApiError catch (e) {
    if (context.mounted) {
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(e.message)));
    }
  }
}

// ---------------------------------------------------------------------------
// VIEW 3 — Hyper-local marketplace (emerald accents)
// ---------------------------------------------------------------------------
class MarketScreen extends ConsumerStatefulWidget {
  const MarketScreen(
      {super.key, required this.township, required this.onChatOpened});
  final String township;
  final void Function(int threadId, String productTitle) onChatOpened;
  @override
  ConsumerState<MarketScreen> createState() => _MarketScreenState();
}

class _MarketScreenState extends ConsumerState<MarketScreen> {
  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  @override
  Widget build(BuildContext context) {
    final market = ref.watch(marketProvider(widget.township));
    final crossAxis =
        MediaQuery.of(context).size.width > 600 ? 3 : 2;
    return Column(children: [
      Container(
        margin: const EdgeInsets.all(12),
        width: double.infinity,
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          gradient: const LinearGradient(
              colors: [NexusColors.market, NexusColors.marketDeep]),
          borderRadius: BorderRadius.circular(16),
        ),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('Hyper-Local Marketplace',
              style: TextStyle(
                  color: Colors.white,
                  fontSize: 15,
                  fontWeight: FontWeight.bold)),
          const Text('Trade items in MMK or Nexus Coins locally.',
              style: TextStyle(color: Colors.white70, fontSize: 10)),
          const SizedBox(height: 8),
          FilledButton.tonal(
            style: FilledButton.styleFrom(
                backgroundColor: Colors.white24,
                foregroundColor: Colors.white,
                padding: const EdgeInsets.symmetric(horizontal: 12)),
            onPressed: _createListing,
            child: const Text('Create listing',
                style: TextStyle(fontSize: 11, fontWeight: FontWeight.bold)),
          ),
        ]),
      ),
      Expanded(
        child: StateSwitch(
          value: market,
          emptyIcon: Icons.storefront_outlined,
          emptyLabel: 'No listings in ${widget.township} yet.',
          onData: (products) => GridView.builder(
            padding: const EdgeInsets.fromLTRB(12, 0, 12, 12),
            gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
                crossAxisCount: crossAxis, childAspectRatio: 0.78,
                crossAxisSpacing: 10, mainAxisSpacing: 10),
            itemCount: products.length,
            itemBuilder: (c, i) => ProductCard(
              product: products[i],
              onBuyChat: () => _buyChat(products[i]),
            ),
          ),
        ),
      ),
    ]);
  }

  Future<void> _buyChat(Product p) async {
    try {
      final t = await ref
          .read(apiProvider)
          .createThread('business', productId: p.id);
      widget.onChatOpened(t.id, p.title);
    } on ApiError catch (e) {
      _toast(e.message);
    }
  }

  Future<void> _createListing() async {
    final title = TextEditingController();
    final price = TextEditingController();
    final ok = await showDialog<bool>(
      context: context,
      builder: (c) => AlertDialog(
        title: const Text('New listing'),
        content: Column(mainAxisSize: MainAxisSize.min, children: [
          TextField(controller: title,
              decoration: const InputDecoration(labelText: 'Title')),
          TextField(controller: price,
              keyboardType: TextInputType.number,
              decoration:
                  const InputDecoration(labelText: 'Price in Nexus Coins')),
        ]),
        actions: [
          TextButton(onPressed: () => Navigator.pop(c, false), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(c, true), child: const Text('List')),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await ref.read(apiProvider).createProduct(
            title: title.text.trim(),
            priceCoins: int.tryParse(price.text.trim()) ?? 0,
            priceMmk: 0,
            township: widget.township,
          );
      ref.invalidate(marketProvider(widget.township));
    } on ApiError catch (e) {
      _toast(e.message);
    }
  }
}

// ---------------------------------------------------------------------------
// VIEW 4 — Unified chat (personal vs business mode)
// ---------------------------------------------------------------------------
class ChatScreen extends ConsumerStatefulWidget {
  const ChatScreen({super.key, required this.openThread});
  final ValueNotifier<int?> openThread;
  @override
  ConsumerState<ChatScreen> createState() => _ChatScreenState();
}

class _ChatScreenState extends ConsumerState<ChatScreen> {
  String _mode = 'personal';
  int? _threadId;
  List<ChatMessage> _messages = [];
  bool _loadingMessages = false;
  final _input = TextEditingController();

  @override
  void initState() {
    super.initState();
    widget.openThread.addListener(_onOpenThread);
  }

  void _onOpenThread() {
    final id = widget.openThread.value;
    if (id != null) {
      setState(() {
        _mode = 'business';
        _threadId = id;
        _loadMessages();
      });
    }
  }

  @override
  void dispose() {
    widget.openThread.removeListener(_onOpenThread);
    _input.dispose();
    super.dispose();
  }

  Future<void> _loadMessages() async {
    if (_threadId == null) return;
    setState(() => _loadingMessages = true);
    try {
      final m = await ref.read(apiProvider).messages(_threadId!);
      if (mounted) setState(() => _messages = m);
    } on ApiError catch (_) {
      if (mounted) setState(() => _messages = []);
    } finally {
      if (mounted) setState(() => _loadingMessages = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final threads = ref.watch(threadsProvider);
    return Card(
      margin: const EdgeInsets.all(12),
      child: Column(children: [
        Padding(
          padding: const EdgeInsets.all(8),
          child: SegmentedButton<String>(
            segments: const [
              ButtonSegment(value: 'personal', label: Text('Personal')),
              ButtonSegment(value: 'business', label: Text('Business')),
            ],
            selected: {_mode},
            onSelectionChanged: (s) => setState(() {
              _mode = s.first;
              _threadId = null;
              _messages = [];
            }),
          ),
        ),
        if (_threadId == null)
          Expanded(
            child: StateSwitch(
              value: threads,
              emptyIcon: Icons.forum_outlined,
              emptyLabel: 'No $_mode chats yet.',
              onData: (list) {
                final filtered = list.where((t) => t.mode == _mode).toList();
                if (filtered.isEmpty) {
                  return EmptyView(
                      icon: Icons.forum_outlined,
                      label: 'No $_mode chats yet.');
                }
                return ListView.builder(
                  itemCount: filtered.length,
                  itemBuilder: (c, i) => ListTile(
                    leading: Icon(
                        _mode == 'business'
                            ? Icons.storefront
                            : Icons.person,
                        color: _mode == 'business'
                            ? NexusColors.market
                            : NexusColors.brand),
                    title: Text(filtered[i].productTitle.isEmpty
                        ? 'Chat #${filtered[i].id}'
                        : filtered[i].productTitle),
                    subtitle: Text(
                        _mode == 'business'
                            ? 'Nexus Coins accepted for payment'
                            : 'Personal chat thread',
                        style: const TextStyle(fontSize: 10)),
                    onTap: () => setState(() {
                      _threadId = filtered[i].id;
                      _loadMessages();
                    }),
                  ),
                );
              },
            ),
          )
        else
          Expanded(
            child: Column(children: [
              ListTile(
                leading: BackButton(onPressed: () => setState(() {
                  _threadId = null;
                  _messages = [];
                })),
                title: const Text('Chat thread'),
                dense: true,
              ),
              Expanded(
                child: _loadingMessages
                    ? const LoadingView()
                    : (_messages.isEmpty
                        ? const EmptyView(
                            icon: Icons.chat_bubble_outline,
                            label: 'No messages yet — say hello!')
                        : ListView.builder(
                            padding: const EdgeInsets.all(12),
                            itemCount: _messages.length,
                            itemBuilder: (c, i) {
                              final mine =
                                  _messages[i].senderId == ref.read(authProvider).userId;
                              return Align(
                                alignment: mine
                                    ? Alignment.centerRight
                                    : Alignment.centerLeft,
                                child: Container(
                                  margin: const EdgeInsets.only(bottom: 6),
                                  padding: const EdgeInsets.all(10),
                                  constraints: const BoxConstraints(maxWidth: 260),
                                  decoration: BoxDecoration(
                                    color: mine
                                        ? NexusColors.brand
                                        : (Theme.of(context).brightness ==
                                                Brightness.dark
                                            ? const Color(0xFF334155)
                                            : const Color(0xFFE2E8F0)),
                                    borderRadius: BorderRadius.circular(16),
                                  ),
                                  child: Text(_messages[i].body,
                                      style: TextStyle(
                                          fontSize: 12,
                                          color: mine
                                              ? Colors.white
                                              : Theme.of(context)
                                                  .colorScheme
                                                  .onSurface)),
                                ),
                              );
                            },
                          )),
              ),
              Padding(
                padding: const EdgeInsets.all(8),
                child: Row(children: [
                  Expanded(
                    child: TextField(
                      controller: _input,
                      decoration: const InputDecoration(hintText: 'Type a message…'),
                    ),
                  ),
                  const SizedBox(width: 8),
                  IconButton.filled(
                    style: IconButton.styleFrom(backgroundColor: NexusColors.brand),
                    onPressed: () async {
                      final text = _input.text.trim();
                      if (text.isEmpty || _threadId == null) return;
                      final messenger = ScaffoldMessenger.of(context);
                      try {
                        await ref.read(apiProvider).sendMessage(_threadId!, text);
                        if (!mounted) return;
                        _input.clear();
                        setState(() => _messages = [
                              ..._messages,
                              ChatMessage.fromJson({
                                'id': DateTime.now().millisecondsSinceEpoch,
                                'sender_id': ref.read(authProvider).userId ?? 0,
                                'body': text,
                              })
                            ]);
                      } on ApiError catch (e) {
                        messenger.showSnackBar(SnackBar(content: Text(e.message)));
                      }
                    },
                    icon: const Icon(Icons.send, color: Colors.white),
                  ),
                ]),
              ),
            ]),
          ),
      ]),
    );
  }
}

// ---------------------------------------------------------------------------
// VIEW 5 — Profile separation dashboard (user vs dating schemas)
// ---------------------------------------------------------------------------
class ProfilesScreen extends ConsumerStatefulWidget {
  const ProfilesScreen({super.key, required this.township});
  final String township;
  @override
  ConsumerState<ProfilesScreen> createState() => _ProfilesScreenState();
}

class _ProfilesScreenState extends ConsumerState<ProfilesScreen> {
  int _tab = 0; // 0 user profile, 1 dating profile
  final _realName = TextEditingController();
  final _nickname = TextEditingController();
  final _bio = TextEditingController();
  bool _hidden = true;
  bool _userFilled = false;
  bool _datingFilled = false;

  @override
  void dispose() {
    _realName.dispose();
    _nickname.dispose();
    _bio.dispose();
    super.dispose();
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(12),
      children: [
        SegmentedButton<int>(
          segments: const [
            ButtonSegment(value: 0, label: Text('User Profile (user_profiles)')),
            ButtonSegment(value: 1, label: Text('Dating (dating_profiles)')),
          ],
          selected: {_tab},
          onSelectionChanged: (s) => setState(() => _tab = s.first),
        ),
        const SizedBox(height: 12),
        const SocialLoginPanel(),
        const SizedBox(height: 12),
        if (_tab == 0) _userCard() else _datingCard(),
      ],
    );
  }

  Widget _userCard() {
    final me = ref.watch(profileProvider);
    return me.when(
      loading: () => const LoadingView(),
      error: (e, _) => ErrorView(message: e.toString(), onRetry: () => ref.invalidate(profileProvider)),
      data: (d) {
        if (!_userFilled) {
          _userFilled = true;
          _realName.text = (d['display_name'] as String?) ?? '';
        }
        return Card(
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              const Chip(label: Text('MAIN ACCOUNT · user_profiles'),
                  avatar: Icon(Icons.person, color: NexusColors.brand, size: 16)),
              const SizedBox(height: 8),
              TextField(
                  controller: _realName,
                  decoration:
                      const InputDecoration(labelText: 'Main Account Real Name')),
              const SizedBox(height: 8),
              TextField(
                  controller: TextEditingController(text: widget.township),
                  readOnly: true,
                  decoration:
                      const InputDecoration(labelText: 'Township Address')),
              const SizedBox(height: 12),
              SizedBox(
                width: double.infinity,
                child: FilledButton(
                  onPressed: () async {
                    try {
                      await ref.read(apiProvider).putProfile(
                          _realName.text.trim(), widget.township, '');
                      _toast('User profile saved');
                      ref.invalidate(profileProvider);
                    } on ApiError catch (e) {
                      _toast(e.message);
                    }
                  },
                  child: const Text('Save User Profile Data'),
                ),
              ),
            ]),
          ),
        );
      },
    );
  }

  Widget _datingCard() {
    final dp = ref.watch(datingProfileProvider);
    return dp.when(
      loading: () => const LoadingView(),
      error: (e, _) => ErrorView(message: e.toString(), onRetry: () => ref.invalidate(datingProfileProvider)),
      data: (d) {
        if (!_datingFilled) {
          _datingFilled = true;
          _nickname.text = (d['nickname'] as String?) ?? '';
          _bio.text = (d['bio'] as String?) ?? '';
          _hidden = (d['is_hidden_from_friends'] as bool?) ?? true;
        }
        return Card(
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              const Chip(label: Text('SEPARATE DATING SCHEMA · dating_profiles'),
                  avatar: Icon(Icons.favorite, color: NexusColors.dating, size: 16)),
              const SizedBox(height: 8),
              TextField(
                  controller: _nickname,
                  decoration:
                      const InputDecoration(labelText: 'Dating Nickname (Isolated)')),
              const SizedBox(height: 8),
              TextField(
                  controller: _bio,
                  maxLines: 3,
                  decoration:
                      const InputDecoration(labelText: 'Dating Bio & Preferences')),
              SwitchListTile(
                value: _hidden,
                onChanged: (v) => setState(() => _hidden = v),
                title: const Text('Hide from Social Friends',
                    style: TextStyle(fontSize: 12)),
                activeThumbColor: NexusColors.dating,
              ),
              const SizedBox(height: 8),
              SizedBox(
                width: double.infinity,
                child: FilledButton(
                  style: FilledButton.styleFrom(backgroundColor: NexusColors.dating),
                  onPressed: () async {
                    try {
                      await ref.read(apiProvider).putDatingProfile(
                          _nickname.text.trim(), _bio.text.trim(), _hidden);
                      _toast('Dating profile saved');
                      ref.invalidate(datingProfileProvider);
                    } on ApiError catch (e) {
                      _toast(e.message);
                    }
                  },
                  child: const Text('Save Dating Profile Data'),
                ),
              ),
            ]),
          ),
        );
      },
    );
  }
}
