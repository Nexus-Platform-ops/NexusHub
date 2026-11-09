import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'api.dart';
import 'state.dart';
import 'theme.dart';

/// Tinder-like card stack with swipe animations.
class SwipeableCardStack extends ConsumerStatefulWidget {
  const SwipeableCardStack({super.key, required this.nodes, required this.onSwipe});
  final List<Map<String, dynamic>> nodes;
  final Future<void> Function(String nodeId, String action) onSwipe;

  @override
  ConsumerState<SwipeableCardStack> createState() => _SwipeableCardStackState();
}

class _SwipeableCardStackState extends ConsumerState<SwipeableCardStack>
    with TickerProviderStateMixin {
  int _currentIndex = 0;
  double _dragX = 0;
  late AnimationController _swipeController;
  late AnimationController _likeController;

  @override
  void initState() {
    super.initState();
    _swipeController = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 300),
    );
    _likeController = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 600),
    );
  }

  @override
  void dispose() {
    _swipeController.dispose();
    _likeController.dispose();
    super.dispose();
  }

  void _onHorizontalDragUpdate(DragUpdateDetails details) {
    setState(() {
      _dragX += details.delta.dx;
    });
  }

  void _onHorizontalDragEnd(DragEndDetails details) {
    final threshold = 100.0;
    if (_dragX.abs() > threshold) {
      final action = _dragX > 0 ? 'like' : 'pass';
      _animateSwipe(action);
    } else {
      setState(() {
        _dragX = 0;
      });
    }
  }

  void _animateSwipe(String action) async {
    final node = widget.nodes[_currentIndex];
    final nodeId = node['node_id'] as String;

    // Animate card off screen
    _swipeController.forward().then((_) {
      _swipeController.reset();
      setState(() {
        _dragX = 0;
      });
    });

    // Show like/pass animation
    if (action == 'like') {
      _likeController.forward().then((_) {
        _likeController.reset();
      });
    }

    try {
      final res = await widget.onSwipe(nodeId, action);
      if (res && mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Matched! Whisper chat unlocked 💬')),
        );
      }
    } on ApiError catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(e.message)),
        );
      }
    }

    if (mounted) {
      setState(() {
        _currentIndex++;
      });
    }
  }

  void _onButtonSwipe(String action) {
    _animateSwipe(action);
  }

  @override
  Widget build(BuildContext context) {
    if (_currentIndex >= widget.nodes.length) {
      return const Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(Icons.favorite_outline, size: 64, color: NexusColors.dating),
            SizedBox(height: 16),
            Text(
              'No more profiles nearby',
              style: TextStyle(fontSize: 16, color: NexusColors.muted),
            ),
            SizedBox(height: 8),
            Text(
              'Check back later for more people',
              style: TextStyle(fontSize: 12, color: NexusColors.muted),
            ),
          ],
        ),
      );
    }

    final node = widget.nodes[_currentIndex];
    final rotation = _dragX / 20;
    final opacity = (_dragX.abs() / 150).clamp(0.0, 1.0);

    return Stack(
      alignment: Alignment.center,
      children: [
        // Card stack (show next card underneath)
        if (_currentIndex + 1 < widget.nodes.length)
          Positioned.fill(
            child: Padding(
              padding: const EdgeInsets.all(24),
              child: _buildCard(widget.nodes[_currentIndex + 1], scale: 0.95),
            ),
          ),

        // Current card with swipe animation
        AnimatedBuilder(
          animation: _swipeController,
          builder: (context, child) {
            final swipeOffset = _swipeController.value *
                (_dragX > 0 ? 500 : -500);
            return Transform.translate(
              offset: Offset(_dragX + swipeOffset, 0),
              child: Transform.rotate(
                angle: rotation * 0.01,
                child: GestureDetector(
                  onHorizontalDragUpdate: _onHorizontalDragUpdate,
                  onHorizontalDragEnd: _onHorizontalDragEnd,
                  child: Padding(
                    padding: const EdgeInsets.all(12),
                    child: _buildCard(node),
                  ),
                ),
              ),
            );
          },
        ),

        // Like/Pass overlay indicators
        if (_dragX.abs() > 50)
          Positioned(
            top: 40,
            left: _dragX > 0 ? 40 : null,
            right: _dragX < 0 ? 40 : null,
            child: Opacity(
              opacity: opacity,
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 10),
                decoration: BoxDecoration(
                  border: Border.all(
                    color: _dragX > 0 ? Colors.green : Colors.red,
                    width: 4,
                  ),
                  borderRadius: BorderRadius.circular(8),
                ),
                child: Text(
                  _dragX > 0 ? 'LIKE' : 'PASS',
                  style: TextStyle(
                    fontSize: 32,
                    fontWeight: FontWeight.bold,
                    color: _dragX > 0 ? Colors.green : Colors.red,
                  ),
                ),
              ),
            ),
          ),

        // Action buttons
        Positioned(
          bottom: 20,
          child: Row(
            mainAxisAlignment: MainAxisAlignment.spaceEvenly,
            children: [
              IconButton.filledTonal(
                onPressed: () => _onButtonSwipe('pass'),
                icon: const Icon(Icons.close, size: 32, color: NexusColors.muted),
              ),
              IconButton.filled(
                style: FilledButton.styleFrom(
                  backgroundColor: NexusColors.dating,
                  minimumSize: const Size(64, 64),
                ),
                onPressed: () => _onButtonSwipe('like'),
                icon: const Icon(Icons.favorite, size: 32, color: Colors.white),
              ),
            ],
          ),
        ),

        // Heart animation for likes
        AnimatedBuilder(
          animation: _likeController,
          builder: (context, child) {
            return Opacity(
              opacity: 1 - _likeController.value,
              child: Transform.scale(
                scale: 1 + _likeController.value * 2,
                child: const Icon(
                  Icons.favorite,
                  size: 100,
                  color: NexusColors.dating,
                ),
              ),
            );
          },
        ),
      ],
    );
  }

  Widget _buildCard(Map<String, dynamic> node, {double scale = 1.0}) {
    return Transform.scale(
      scale: scale,
      child: Card(
        clipBehavior: Clip.antiAlias,
        elevation: 8,
        child: Column(
          children: [
            Expanded(
              child: Container(
                width: double.infinity,
                decoration: BoxDecoration(
                  gradient: LinearGradient(
                    begin: Alignment.topLeft,
                    end: Alignment.bottomRight,
                    colors: [
                      NexusColors.dating.withValues(alpha: 0.3),
                      NexusColors.dating.withValues(alpha: 0.1),
                    ],
                  ),
                ),
                child: Center(
                  child: Column(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      CircleAvatar(
                        radius: 60,
                        backgroundColor: Colors.white.withValues(alpha: 0.3),
                        child: Icon(
                          Icons.person,
                          size: 60,
                          color: NexusColors.dating,
                        ),
                      ),
                      const SizedBox(height: 16),
                      Text(
                        '${node['handle']}',
                        style: const TextStyle(
                          fontSize: 24,
                          fontWeight: FontWeight.bold,
                          color: Colors.white,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
            Container(
              padding: const EdgeInsets.all(16),
              child: Column(
                children: [
                  Text(
                    '${node['distance_bucket']} away',
                    style: TextStyle(
                      fontSize: 14,
                      color: NexusColors.muted,
                    ),
                  ),
                  const SizedBox(height: 8),
                  Row(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      TextButton.icon(
                        onPressed: () {
                          // Report functionality
                        },
                        icon: const Icon(Icons.flag_outlined, size: 16),
                        label: const Text('Report'),
                      ),
                      const SizedBox(width: 16),
                      TextButton.icon(
                        onPressed: () {
                          // Block functionality
                        },
                        icon: const Icon(Icons.block, size: 16),
                        label: const Text('Block'),
                      ),
                    ],
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Animated radar with rotating scan effect.
class AnimatedRadar extends StatefulWidget {
  const AnimatedRadar({super.key, required this.nodes});
  final List<Map<String, dynamic>> nodes;

  @override
  State<AnimatedRadar> createState() => _AnimatedRadarState();
}

class _AnimatedRadarState extends State<AnimatedRadar>
    with TickerProviderStateMixin {
  late AnimationController _rotationController;

  @override
  void initState() {
    super.initState();
    _rotationController = AnimationController(
      vsync: this,
      duration: const Duration(seconds: 4),
    )..repeat();
  }

  @override
  void dispose() {
    _rotationController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          SizedBox(
            width: 280,
            height: 280,
            child: Stack(
              alignment: Alignment.center,
              children: [
                // Outer ring
                Container(
                  width: 280,
                  height: 280,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    border: Border.all(
                      color: NexusColors.dating.withValues(alpha: 0.3),
                      width: 2,
                    ),
                  ),
                ),
                // Middle ring
                Container(
                  width: 200,
                  height: 200,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    border: Border.all(
                      color: NexusColors.dating.withValues(alpha: 0.4),
                      width: 2,
                    ),
                  ),
                ),
                // Inner ring with gradient
                Container(
                  width: 120,
                  height: 120,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    gradient: RadialGradient(
                      colors: [
                        NexusColors.dating.withValues(alpha: 0.2),
                        NexusColors.dating.withValues(alpha: 0.05),
                      ],
                    ),
                    border: Border.all(
                      color: NexusColors.dating.withValues(alpha: 0.5),
                      width: 2,
                    ),
                  ),
                  child: const Center(
                    child: Icon(
                      Icons.favorite,
                      color: NexusColors.dating,
                      size: 48,
                    ),
                  ),
                ),
                // Rotating scan line
                AnimatedBuilder(
                  animation: _rotationController,
                  builder: (context, child) {
                    return Transform.rotate(
                      angle: _rotationController.value * 2 * 3.14159,
                      child: CustomPaint(
                        size: const Size(280, 280),
                        painter: _RadarScanPainter(),
                      ),
                    );
                  },
                ),
                // Node markers
                ...widget.nodes.asMap().entries.map((entry) {
                  final i = entry.key;
                  final node = entry.value;
                  return _RadarNode(
                    node: node,
                    index: i,
                    total: widget.nodes.length,
                  );
                }),
              ],
            ),
          ),
          const SizedBox(height: 24),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 32),
            child: Text(
              widget.nodes.isEmpty
                  ? 'Scanning for people nearby…'
                  : '${widget.nodes.length} people nearby',
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 14,
                color: NexusColors.muted,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _RadarNode extends StatelessWidget {
  const _RadarNode({
    required this.node,
    required this.index,
    required this.total,
  });

  final Map<String, dynamic> node;
  final int index;
  final int total;

  @override
  Widget build(BuildContext context) {
    // Distribute nodes around the circle
    final angle = (index / total) * 2 * 3.14159;
    final radius = 110.0;
    final x = radius * cos(angle);
    final y = radius * sin(angle);

    return Positioned(
      left: 140 + x - 20,
      top: 140 + y - 20,
      child: GestureDetector(
        onLongPress: () {
          // Show node actions
        },
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            CircleAvatar(
              radius: 20,
              backgroundColor: NexusColors.dating.withValues(alpha: 0.3),
              child: const Icon(
                Icons.person,
                size: 20,
                color: NexusColors.dating,
              ),
            ),
            const SizedBox(height: 4),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
              decoration: BoxDecoration(
                color: Colors.black.withValues(alpha: 0.7),
                borderRadius: BorderRadius.circular(8),
              ),
              child: Text(
                '${node['handle']}',
                style: const TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.bold,
                  color: Colors.white,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _RadarScanPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width / 2, size.height / 2);
    final radius = size.width / 2;

    final paint = Paint()
      ..shader = Gradient.sweep(
        center,
        [
          NexusColors.dating.withValues(alpha: 0.4),
          NexusColors.dating.withValues(alpha: 0.0),
        ],
      )
      ..style = PaintingStyle.fill;

    canvas.drawCircle(center, radius, paint);
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => true;
}

// Helper for cos/sin
double cos(double x) => x.cos;
double sin(double x) => x.sin;

extension on double {
  double get cos => _cos(this);
  double get sin => _sin(this);
}

double _cos(double x) {
  // Simple approximation
  var result = 1.0;
  var term = 1.0;
  for (int i = 1; i <= 10; i++) {
    term *= -x * x / ((2 * i - 1) * (2 * i));
    result += term;
  }
  return result;
}

double _sin(double x) {
  // Simple approximation
  var result = x;
  var term = x;
  for (int i = 1; i <= 10; i++) {
    term *= -x * x / ((2 * i) * (2 * i + 1));
    result += term;
  }
  return result;
}
