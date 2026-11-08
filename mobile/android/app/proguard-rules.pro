# Nexus App — R8/ProGuard rules (release builds).
#
# Crypto and WebRTC keeps are prepared per the build spec; neither plugin is
# in pubspec.yaml yet, so the -dontwarn entries keep R8 quiet until they land.

# Flutter embedding
-keep class io.flutter.app.** { *; }
-keep class io.flutter.plugin.** { *; }
-keep class io.flutter.util.** { *; }
-keep class io.flutter.view.** { *; }
-keep class io.flutter.** { *; }
-keep class io.flutter.plugins.** { *; }
-keep class io.flutter.plugin.editing.** { *; }

# JSON / reflection used by http payloads and codegen-free models
-keepattributes Signature, *Annotation*, EnclosingMethod, InnerClasses
-keepclassmembers class * {
    @com.google.gson.annotations.SerializedName <fields>;
}
-keepclassmembers enum * {
    public static **[] values();
    public static ** valueOf(java.lang.String);
}

# Sentry Flutter SDK
-keepattributes LineNumberTable, SourceFile
-renamesourcefileattribute SourceFile
-keep class io.sentry.** { *; }
-keep class io.sentry.android.** { *; }
-dontwarn io.sentry.**

# Crypto (prepared for payments / token storage)
-keep class javax.crypto.** { *; }
-keep class javax.net.ssl.** { *; }
-dontwarn javax.crypto.**
-dontwarn org.conscrypt.**
-dontwarn org.bouncycastle.**
-dontwarn org.openjsse.**

# WebRTC (prepared for voice/video)
-keep class org.webrtc.** { *; }
-keep class org.webrtc.audio.** { *; }
-dontwarn org.webrtc.**

# Flutter deferred components reference Play Core split APIs that the app
# never ships; R8 flags them as missing without these.
-dontwarn com.google.android.play.core.splitcompat.SplitCompatApplication
-dontwarn com.google.android.play.core.splitinstall.SplitInstallException
-dontwarn com.google.android.play.core.splitinstall.SplitInstallManager
-dontwarn com.google.android.play.core.splitinstall.SplitInstallManagerFactory
-dontwarn com.google.android.play.core.splitinstall.SplitInstallRequest$Builder
-dontwarn com.google.android.play.core.splitinstall.SplitInstallRequest
-dontwarn com.google.android.play.core.splitinstall.SplitInstallSessionState
-dontwarn com.google.android.play.core.splitinstall.SplitInstallStateUpdatedListener
-dontwarn com.google.android.play.core.tasks.OnFailureListener
-dontwarn com.google.android.play.core.tasks.OnSuccessListener
-dontwarn com.google.android.play.core.tasks.Task
