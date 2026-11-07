import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'api.dart';
import 'theme.dart';

/// Loading / error / empty states shared by every screen (G3 requirement).
class LoadingView extends StatelessWidget {
  const LoadingView({super.key, this.label});
  final String? label;
  @override
  Widget build(BuildContext context) => Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const CircularProgressIndicator(color: NexusColors.brand),
            if (label != null) ...[
              const SizedBox(height: 12),
              Text(label!, style: Theme.of(context).textTheme.bodySmall),
            ],
          ],
        ),
      );
}

class ErrorView extends StatelessWidget {
  const ErrorView({super.key, required this.message, this.onRetry});
  final String message;
  final VoidCallback? onRetry;
  @override
  Widget build(BuildContext context) => Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.cloud_off, size: 40, color: NexusColors.muted),
              const SizedBox(height: 12),
              Text(message,
                  textAlign: TextAlign.center,
                  style: Theme.of(context).textTheme.bodySmall),
              const SizedBox(height: 12),
              FilledButton(
                onPressed: onRetry,
                child: const Text('Retry'),
              ),
            ],
          ),
        ),
      );
}

class EmptyView extends StatelessWidget {
  const EmptyView({super.key, required this.icon, required this.label});
  final IconData icon;
  final String label;
  @override
  Widget build(BuildContext context) => Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(icon, size: 40, color: NexusColors.muted),
              const SizedBox(height: 12),
              Text(label,
                  textAlign: TextAlign.center,
                  style: Theme.of(context).textTheme.bodySmall),
            ],
          ),
        ),
      );
}

/// Renders a FutureProvider value with the three standard states.
class StateSwitch<T> extends StatelessWidget {
  const StateSwitch({
    super.key,
    required this.value,
    required this.onData,
    this.emptyIcon = Icons.inbox,
    this.emptyLabel = 'Nothing here yet',
    this.skipEmptyWhen,
  });
  final AsyncValue<T> value;
  final Widget Function(T data) onData;
  final IconData emptyIcon;
  final String emptyLabel;
  final bool Function(T data)? skipEmptyWhen;

  @override
  Widget build(BuildContext context) {
    return value.when(
      loading: () => const LoadingView(),
      error: (e, _) => ErrorView(
        message: e is ApiError
            ? (e.status == 404 ? emptyLabel : e.message)
            : e.toString(),
        onRetry: () {},
      ),
      data: (d) =>
          (d is List && d.isEmpty || (skipEmptyWhen != null && skipEmptyWhen!(d)))
              ? EmptyView(icon: emptyIcon, label: emptyLabel)
              : onData(d),
    );
  }
}

class PostCard extends StatelessWidget {
  const PostCard({super.key, required this.post, this.onLike});
  final Post post;
  final VoidCallback? onLike;
  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    return Card(
      margin: const EdgeInsets.only(bottom: 12),
      child: Padding(
        padding: const EdgeInsets.all(14),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(children: [
              CircleAvatar(
                radius: 16,
                backgroundColor: NexusColors.brand.withValues(alpha: 0.2),
                child: Text(post.author.isNotEmpty ? post.author[0] : '?',
                    style: const TextStyle(
                        fontSize: 12, fontWeight: FontWeight.bold,
                        color: NexusColors.brand700)),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Column(crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                  Text(post.author,
                      style: const TextStyle(
                          fontSize: 12, fontWeight: FontWeight.bold)),
                  Text(post.township,
                      style: TextStyle(
                          fontSize: 10, color: NexusColors.muted)),
                ]),
              ),
            ]),
            const SizedBox(height: 10),
            Text(post.content,
                style: TextStyle(
                    fontSize: 12,
                    color: cs.onSurface.withValues(alpha: 0.87))),
            const Divider(height: 20),
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceAround,
              children: [
                _Action(icon: Icons.thumb_up_alt_outlined, label: 'Like',
                    onTap: onLike,
                    color: NexusColors.brand),
                const _Action(icon: Icons.comment_outlined, label: 'Comment'),
                const _Action(icon: Icons.share_outlined, label: 'Share'),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

class _Action extends StatelessWidget {
  const _Action({required this.icon, required this.label, this.onTap, this.color});
  final IconData icon;
  final String label;
  final VoidCallback? onTap;
  final Color? color;
  @override
  Widget build(BuildContext context) => InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(8),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
          child: Row(mainAxisSize: MainAxisSize.min, children: [
            Icon(icon, size: 16, color: color ?? NexusColors.muted),
            const SizedBox(width: 5),
            Text(label,
                style: TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w600,
                    color: color ?? NexusColors.muted)),
          ]),
        ),
      );
}

class ProductCard extends StatelessWidget {
  const ProductCard({super.key, required this.product, this.onBuyChat});
  final Product product;
  final VoidCallback? onBuyChat;
  @override
  Widget build(BuildContext context) => Card(
        child: Padding(
          padding: const EdgeInsets.all(10),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              ClipRRect(
                borderRadius: BorderRadius.circular(12),
                child: Container(
                  height: 96,
                  width: double.infinity,
                  color: NexusColors.market.withValues(alpha: 0.12),
                  child: const Icon(Icons.inventory_2_outlined,
                      color: NexusColors.market),
                ),
              ),
              const SizedBox(height: 8),
              Text(product.title,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                      fontSize: 12, fontWeight: FontWeight.bold)),
              const SizedBox(height: 2),
              Text('${product.priceCoins} Coins',
                  style: const TextStyle(
                      fontSize: 10,
                      fontWeight: FontWeight.bold,
                      color: NexusColors.brand600)),
              const SizedBox(height: 8),
              SizedBox(
                width: double.infinity,
                child: FilledButton(
                  style: FilledButton.styleFrom(
                    backgroundColor:
                        Theme.of(context).brightness == Brightness.dark
                            ? NexusColors.brand
                            : const Color(0xFF0F172A),
                    padding: const EdgeInsets.symmetric(vertical: 6),
                    textStyle: const TextStyle(
                        fontSize: 10, fontWeight: FontWeight.bold),
                  ),
                  onPressed: onBuyChat,
                  child: const Text('Buy / Chat'),
                ),
              ),
            ],
          ),
        ),
      );
}
