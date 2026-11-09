import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'api.dart';
import 'state.dart';
import 'theme.dart';

final plansProvider = FutureProvider<List<SubscriptionPlan>>((ref) {
  return ref.read(apiProvider).subscriptionPlans();
});

final mySubscriptionProvider =
    FutureProvider.autoDispose<Subscription?>((ref) {
  final auth = ref.watch(authProvider);
  if (!auth.verified) return null;
  return ref.read(apiProvider).mySubscription();
});

class PaywallScreen extends ConsumerStatefulWidget {
  const PaywallScreen({super.key});
  @override
  ConsumerState<PaywallScreen> createState() => _PaywallScreenState();
}

class _PaywallScreenState extends ConsumerState<PaywallScreen> {
  bool _purchasing = false;
  String? _error;

  Future<void> _purchase(SubscriptionPlan plan) async {
    setState(() {
      _purchasing = true;
      _error = null;
    });
    try {
      final api = ref.read(apiProvider);
      // In production, this would invoke StoreKit 2 / Google Billing Library
      // to get a real receipt token. For now, we simulate with a stub.
      final receipt = 'stub_receipt_${DateTime.now().millisecondsSinceEpoch}';
      final platform = Theme.of(context).platform == TargetPlatform.iOS
          ? 'apple'
          : 'google';
      await api.createSubscription(
        platform: platform,
        receiptToken: receipt,
        planId: plan.id,
      );
      ref.invalidate(mySubscriptionProvider);
      if (mounted) {
        Navigator.of(context).pop();
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Nexus+ activated: ${plan.name}')),
        );
      }
    } on ApiError catch (e) {
      setState(() => _error = e.message);
    } finally {
      if (mounted) setState(() => _purchasing = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final plans = ref.watch(plansProvider);
    final currentSub = ref.watch(mySubscriptionProvider);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Nexus+'),
        actions: [
          if (currentSub.valueOrNull?.isActive == true)
            TextButton(
              onPressed: () => Navigator.pop(context),
              child: const Text('Current Plan'),
            ),
        ],
      ),
      body: plans.when(
        data: (planList) => _buildContent(planList, currentSub.valueOrNull),
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text('Error: $e')),
      ),
    );
  }

  Widget _buildContent(
      List<SubscriptionPlan> planList, Subscription? currentSub) {
    if (currentSub?.isActive == true) {
      return _buildActiveSubscription(currentSub!);
    }
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        const _PaywallHeader(),
        const SizedBox(height: 24),
        ...planList.map((plan) => _PlanCard(
              plan: plan,
              purchasing: _purchasing,
              onPurchase: () => _purchase(plan),
            )),
        if (_error != null) ...[
          const SizedBox(height: 16),
          Text(_error!, style: const TextStyle(color: Colors.red)),
        ],
        const SizedBox(height: 24),
        const _FeatureList(),
        const SizedBox(height: 16),
        Center(
          child: Text(
            'Cancel anytime. Payment charged via your app store account.',
            style: Theme.of(context).textTheme.bodySmall,
            textAlign: TextAlign.center,
          ),
        ),
      ],
    );
  }

  Widget _buildActiveSubscription(Subscription sub) {
    return Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        children: [
          const Icon(Icons.check_circle, color: NexusColors.brand, size: 64),
          const SizedBox(height: 16),
          Text('You have Nexus+',
              style: Theme.of(context).textTheme.headlineSmall),
          const SizedBox(height: 8),
          Text('Plan: ${sub.planId}'),
          Text('Status: ${sub.status}'),
          if (sub.currentPeriodEnd.isNotEmpty)
            Text('Renews: ${sub.currentPeriodEnd}'),
          const SizedBox(height: 24),
          if (!sub.cancelAtPeriodEnd)
            OutlinedButton(
              onPressed: () async {
                try {
                  await ref.read(apiProvider).cancelSubscription();
                  ref.invalidate(mySubscriptionProvider);
                  if (mounted) {
                    ScaffoldMessenger.of(context).showSnackBar(
                      const SnackBar(content: Text('Subscription canceled')),
                    );
                  }
                } on ApiError catch (e) {
                  if (mounted) {
                    ScaffoldMessenger.of(context).showSnackBar(
                      SnackBar(content: Text(e.message)),
                    );
                  }
                }
              },
              style: OutlinedButton.styleFrom(foregroundColor: Colors.red),
              child: const Text('Cancel Subscription'),
            ),
        ],
      ),
    );
  }
}

class _PaywallHeader extends StatelessWidget {
  const _PaywallHeader();

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Container(
          padding: const EdgeInsets.all(16),
          decoration: BoxDecoration(
            gradient: LinearGradient(
              colors: [NexusColors.brand, NexusColors.brand.withOpacity(0.7)],
            ),
            borderRadius: BorderRadius.circular(16),
          ),
          child: const Row(
            children: [
              Icon(Icons.star, color: Colors.white, size: 32),
              SizedBox(width: 12),
              Expanded(
                child: Text(
                  'Unlock Nexus+',
                  style: TextStyle(
                    color: Colors.white,
                    fontSize: 24,
                    fontWeight: FontWeight.bold,
                  ),
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 12),
        Text(
          'Get more out of Nexus with premium features',
          style: Theme.of(context).textTheme.bodyLarge,
          textAlign: TextAlign.center,
        ),
      ],
    );
  }
}

class _PlanCard extends StatelessWidget {
  const _PlanCard({
    required this.plan,
    required this.purchasing,
    required this.onPurchase,
  });
  final SubscriptionPlan plan;
  final bool purchasing;
  final VoidCallback onPurchase;

  @override
  Widget build(BuildContext context) {
    final isMonthly = plan.id.contains('monthly');
    return Card(
      margin: const EdgeInsets.only(bottom: 12),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(plan.name,
                      style: Theme.of(context).textTheme.titleMedium),
                ),
                if (!isMonthly)
                  Container(
                    padding:
                        const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                    decoration: BoxDecoration(
                      color: NexusColors.brand.withOpacity(0.1),
                      borderRadius: BorderRadius.circular(8),
                    ),
                    child: const Text('BEST VALUE',
                        style: TextStyle(
                            color: NexusColors.brand,
                            fontSize: 10,
                            fontWeight: FontWeight.bold)),
                  ),
              ],
            ),
            const SizedBox(height: 8),
            Wrap(
              spacing: 8,
              children: plan.features
                  .map((f) => Chip(
                        label: Text(_featureLabel(f),
                            style: const TextStyle(fontSize: 12)),
                        visualDensity: VisualDensity.compact,
                      ))
                  .toList(),
            ),
            const SizedBox(height: 12),
            SizedBox(
              width: double.infinity,
              child: ElevatedButton(
                onPressed: purchasing ? null : onPurchase,
                style: ElevatedButton.styleFrom(
                  backgroundColor: NexusColors.brand,
                  foregroundColor: Colors.white,
                  padding: const EdgeInsets.symmetric(vertical: 12),
                ),
                child: purchasing
                    ? const SizedBox(
                        width: 20,
                        height: 20,
                        child: CircularProgressIndicator(
                            strokeWidth: 2, color: Colors.white))
                    : Text('Subscribe — ${plan.duration}'),
              ),
            ),
          ],
        ),
      ),
    );
  }

  String _featureLabel(String feature) {
    switch (feature) {
      case 'radius_expansion':
        return 'Extended Radius';
      case 'who_liked_me':
        return 'Who Liked Me';
      case 'profile_boost':
        return 'Profile Boost';
      default:
        return feature;
    }
  }
}

class _FeatureList extends StatelessWidget {
  const _FeatureList();

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('What you get',
            style: Theme.of(context).textTheme.titleMedium),
        const SizedBox(height: 12),
        _featureRow(Icons.explore, 'Radius Expansion',
            'See people in nearby townships, not just your own'),
        _featureRow(Icons.favorite, 'Who Liked Me',
            'See who already swiped right on your profile'),
        _featureRow(Icons.bolt, 'Profile Boost',
            'Get 10x more profile views for 30 minutes'),
        _featureRow(Icons.verified, 'Priority Support',
            'Fast-track response from our support team'),
      ],
    );
  }

  Widget _featureRow(IconData icon, String title, String subtitle) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Row(
        children: [
          Icon(icon, color: NexusColors.brand, size: 24),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(title,
                    style: const TextStyle(fontWeight: FontWeight.w600)),
                Text(subtitle,
                    style: TextStyle(
                        color: Colors.grey[600], fontSize: 13)),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
