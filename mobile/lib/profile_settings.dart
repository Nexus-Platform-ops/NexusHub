import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'api.dart';
import 'state.dart';
import 'theme.dart';

/// Professional profile page with tabs for posts, friends, and about.
class ProfileScreen extends ConsumerStatefulWidget {
  const ProfileScreen({super.key, this.userId});
  final int64? userId; // null = current user

  @override
  ConsumerState<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends ConsumerState<ProfileScreen>
    with SingleTickerProviderStateMixin {
  late TabController _tabController;
  int _tabIndex = 0;

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 3, vsync: this);
    _tabController.addListener(() {
      if (_tabController.indexIsChanging) return;
      setState(() => _tabIndex = _tabController.index);
    });
  }

  @override
  void dispose() {
    _tabController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final isMe = widget.userId == null;
    final profile = ref.watch(profileProvider);

    return Scaffold(
      body: profile.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(child: Text('Error: $e')),
        data: (data) {
          final displayName = data['display_name'] as String? ?? '';
          final phone = data['phone'] as String? ?? '';
          final avatarUrl = data['avatar_url'] as String? ?? '';
          final realName = data['real_name'] as String? ?? '';
          final township = data['township'] as String? ?? '';

          return NestedScrollView(
            headerSliverBuilder: (context, innerBoxIsScrolled) => [
              SliverAppBar(
                expandedHeight: 200,
                pinned: true,
                flexibleSpace: FlexibleSpaceBar(
                  background: Container(
                    decoration: BoxDecoration(
                      gradient: LinearGradient(
                        begin: Alignment.topCenter,
                        end: Alignment.bottomCenter,
                        colors: [
                          NexusColors.brand,
                          NexusColors.brand700,
                        ],
                      ),
                    ),
                    child: Center(
                      child: Column(
                        mainAxisAlignment: MainAxisAlignment.center,
                        children: [
                          CircleAvatar(
                            radius: 50,
                            backgroundColor: Colors.white,
                            backgroundImage: avatarUrl.isNotEmpty
                                ? NetworkImage(avatarUrl)
                                : null,
                            child: avatarUrl.isEmpty
                                ? Text(
                                    displayName.isNotEmpty ? displayName[0] : 'U',
                                    style: TextStyle(
                                      fontSize: 40,
                                      color: NexusColors.brand,
                                      fontWeight: FontWeight.bold,
                                    ),
                                  )
                                : null,
                          ),
                          const SizedBox(height: 12),
                          Text(
                            displayName,
                            style: const TextStyle(
                              fontSize: 24,
                              fontWeight: FontWeight.bold,
                              color: Colors.white,
                            ),
                          ),
                          if (township.isNotEmpty)
                            Text(
                              '📍 $township',
                              style: TextStyle(
                                fontSize: 14,
                                color: Colors.white.withValues(alpha: 0.9),
                              ),
                            ),
                        ],
                      ),
                    ),
                  ),
                ),
                bottom: TabBar(
                  controller: _tabController,
                  tabs: const [
                    Tab(text: 'Posts', icon: Icon(Icons.article)),
                    Tab(text: 'Friends', icon: Icon(Icons.people)),
                    Tab(text: 'About', icon: Icon(Icons.info)),
                  ],
                  indicatorColor: Colors.white,
                  labelColor: Colors.white,
                  unselectedLabelColor: Colors.white54,
                ),
              ),
            ],
            body: TabBarView(
              controller: _tabController,
              children: [
                _PostsTab(userId: widget.userId),
                _FriendsTab(userId: widget.userId),
                _AboutTab(
                  displayName: displayName,
                  phone: phone,
                  realName: realName,
                  township: township,
                  isMe: isMe,
                ),
              ],
            ),
          );
        },
      ),
    );
  }
}

class _PostsTab extends ConsumerWidget {
  const _PostsTab({this.userId});
  final int64? userId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // For now, show empty state - would need a posts by user endpoint
    return const Center(
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Icon(Icons.article_outlined, size: 64, color: NexusColors.muted),
          SizedBox(height: 16),
          Text(
            'Posts will appear here',
            style: TextStyle(fontSize: 16, color: NexusColors.muted),
          ),
        ],
      ),
    );
  }
}

class _FriendsTab extends ConsumerWidget {
  const _FriendsTab({this.userId});
  final int64? userId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // For now, show empty state - would need a friends list endpoint
    return const Center(
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Icon(Icons.people_outline, size: 64, color: NexusColors.muted),
          SizedBox(height: 16),
          Text(
            'Friends will appear here',
            style: TextStyle(fontSize: 16, color: NexusColors.muted),
          ),
        ],
      ),
    );
  }
}

class _AboutTab extends StatelessWidget {
  const _AboutTab({
    required this.displayName,
    required this.phone,
    required this.realName,
    required this.township,
    required this.isMe,
  });

  final String displayName;
  final String phone;
  final String realName;
  final String township;
  final bool isMe;

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        _InfoCard(
          icon: Icons.person,
          title: 'Display Name',
          value: displayName,
        ),
        const SizedBox(height: 12),
        if (realName.isNotEmpty)
          _InfoCard(
            icon: Icons.badge,
            title: 'Real Name',
            value: realName,
          ),
        if (realName.isNotEmpty) const SizedBox(height: 12),
        _InfoCard(
          icon: Icons.phone,
          title: 'Phone',
          value: phone,
        ),
        const SizedBox(height: 12),
        if (township.isNotEmpty)
          _InfoCard(
            icon: Icons.location_city,
            title: 'Township',
            value: township,
          ),
        if (township.isNotEmpty) const SizedBox(height: 12),
        if (isMe) ...[
          const SizedBox(height: 24),
          FilledButton.icon(
            onPressed: () {
              // Navigate to edit profile
            },
            icon: const Icon(Icons.edit),
            label: const Text('Edit Profile'),
          ),
        ],
      ],
    );
  }
}

class _InfoCard extends StatelessWidget {
  const _InfoCard({
    required this.icon,
    required this.title,
    required this.value,
  });

  final IconData icon;
  final String title;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: ListTile(
        leading: Icon(icon, color: NexusColors.brand),
        title: Text(title, style: TextStyle(fontSize: 12, color: NexusColors.muted)),
        subtitle: Text(value, style: const TextStyle(fontSize: 16)),
      ),
    );
  }
}

/// Settings page with notification preferences, privacy controls, and logout.
class SettingsScreen extends ConsumerStatefulWidget {
  const SettingsScreen({super.key});

  @override
  ConsumerState<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends ConsumerState<SettingsScreen> {
  bool _notifyLikes = true;
  bool _notifyComments = true;
  bool _notifyMatches = true;
  bool _notifyMessages = true;
  bool _notifyMarket = true;
  bool _profilePublic = true;
  bool _showOnlineStatus = true;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Settings'),
        backgroundColor: NexusColors.brand,
        foregroundColor: Colors.white,
      ),
      body: ListView(
        children: [
          const _SectionHeader(title: 'Notifications'),
          SwitchListTile(
            title: const Text('Post Likes'),
            subtitle: const Text('When someone likes your post'),
            value: _notifyLikes,
            onChanged: (v) => setState(() => _notifyLikes = v),
            activeColor: NexusColors.brand,
          ),
          SwitchListTile(
            title: const Text('Comments'),
            subtitle: const Text('When someone comments on your post'),
            value: _notifyComments,
            onChanged: (v) => setState(() => _notifyComments = v),
            activeColor: NexusColors.brand,
          ),
          SwitchListTile(
            title: const Text('Dating Matches'),
            subtitle: const Text('When you get a new match'),
            value: _notifyMatches,
            onChanged: (v) => setState(() => _notifyMatches = v),
            activeColor: NexusColors.brand,
          ),
          SwitchListTile(
            title: const Text('Messages'),
            subtitle: const Text('When you receive a new message'),
            value: _notifyMessages,
            onChanged: (v) => setState(() => _notifyMessages = v),
            activeColor: NexusColors.brand,
          ),
          SwitchListTile(
            title: const Text('Market Activity'),
            subtitle: const Text('Sales and inquiries'),
            value: _notifyMarket,
            onChanged: (v) => setState(() => _notifyMarket = v),
            activeColor: NexusColors.brand,
          ),
          const Divider(),
          const _SectionHeader(title: 'Privacy'),
          SwitchListTile(
            title: const Text('Public Profile'),
            subtitle: const Text('Allow others to see your profile'),
            value: _profilePublic,
            onChanged: (v) => setState(() => _profilePublic = v),
            activeColor: NexusColors.brand,
          ),
          SwitchListTile(
            title: const Text('Show Online Status'),
            subtitle: const Text('Let others see when you\'re online'),
            value: _showOnlineStatus,
            onChanged: (v) => setState(() => _showOnlineStatus = v),
            activeColor: NexusColors.brand,
          ),
          const Divider(),
          const _SectionHeader(title: 'Account'),
          ListTile(
            leading: const Icon(Icons.logout, color: Colors.red),
            title: const Text('Logout', style: TextStyle(color: Colors.red)),
            onTap: () => _showLogoutDialog(context),
          ),
          const SizedBox(height: 32),
        ],
      ),
    );
  }

  void _showLogoutDialog(BuildContext context) {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Logout'),
        content: const Text('Are you sure you want to logout?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () {
              ref.read(authProvider.notifier).logout();
              Navigator.pop(context);
            },
            style: FilledButton.styleFrom(backgroundColor: Colors.red),
            child: const Text('Logout'),
          ),
        ],
      ),
    );
  }
}

class _SectionHeader extends StatelessWidget {
  const _SectionHeader({required this.title});
  final String title;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
      child: Text(
        title,
        style: TextStyle(
          fontSize: 14,
          fontWeight: FontWeight.bold,
          color: NexusColors.brand,
        ),
      ),
    );
  }
}
