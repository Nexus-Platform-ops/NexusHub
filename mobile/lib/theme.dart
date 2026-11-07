import 'package:flutter/material.dart';

/// Design tokens extracted from the Myanmar SuperApp prototype (Tailwind
/// palette): brand amber for the main app, rose for dating, emerald for
/// market, purple for whisper, indigo for chat, cyan for profiles.
class NexusColors {
  static const brand = Color(0xFFF59E0B); // amber-500
  static const brand600 = Color(0xFFD97706); // amber-600
  static const brand700 = Color(0xFFB45309); // amber-700
  static const dating = Color(0xFFF43F5E); // rose-500
  static const market = Color(0xFF10B981); // emerald-500
  static const marketDeep = Color(0xFF0F766E); // teal-700
  static const whisper = Color(0xFF9333EA); // purple-600
  static const chat = Color(0xFF6366F1); // indigo-500
  static const profile = Color(0xFF06B6D4); // cyan-500
  static const bgLight = Color(0xFFF1F5F9); // slate-100
  static const bgDark = Color(0xFF0F172A); // slate-900
  static const cardDark = Color(0xFF1E293B); // slate-800
  static const cardLight = Color(0xFFFFFFFF);
  static const muted = Color(0xFF94A3B8); // slate-400
}

ThemeData nexusTheme(Brightness brightness) {
  final isDark = brightness == Brightness.dark;
  final scheme = ColorScheme.fromSeed(
    seedColor: NexusColors.brand,
    brightness: brightness,
  );
  return ThemeData(
    useMaterial3: true,
    colorScheme: scheme,
    scaffoldBackgroundColor: isDark ? NexusColors.bgDark : NexusColors.bgLight,
    fontFamilyFallback: const ['Roboto', 'Noto Sans Myanmar'],
    cardTheme: CardThemeData(
      color: isDark ? NexusColors.cardDark : NexusColors.cardLight,
      elevation: 1,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
    ),
    appBarTheme: AppBarTheme(
      backgroundColor:
          isDark ? NexusColors.cardDark : Colors.white.withValues(alpha: 0.94),
      elevation: 0,
      centerTitle: false,
      foregroundColor: isDark ? Colors.white : const Color(0xFF0F172A),
    ),
    navigationBarTheme: NavigationBarThemeData(
      backgroundColor: isDark ? NexusColors.cardDark : Colors.white,
      indicatorColor: NexusColors.brand.withValues(alpha: 0.18),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        backgroundColor: NexusColors.brand,
        foregroundColor: Colors.white,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
      ),
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: isDark ? const Color(0xFF334155) : const Color(0xFFE2E8F0),
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(24),
        borderSide: BorderSide.none,
      ),
      contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
    ),
    snackBarTheme: const SnackBarThemeData(behavior: SnackBarBehavior.floating),
  );
}
