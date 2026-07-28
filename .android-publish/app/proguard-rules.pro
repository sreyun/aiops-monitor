# AIOps — R8 / ProGuard
-dontwarn okhttp3.**
-dontwarn retrofit2.**
-dontwarn com.google.gson.**

# Keep Retrofit / Gson models & interfaces
-keepattributes Signature
-keepattributes *Annotation*
-keep class com.aiops.monitor.data.models.** { *; }
-keep class com.aiops.monitor.data.ApiService { *; }
-keepclassmembers,allowobfuscation interface * {
    @retrofit2.http.* <methods>;
}
-keep class com.google.gson.** { *; }
-keep class * implements com.google.gson.TypeAdapterFactory
-keep class * implements com.google.gson.JsonSerializer
-keep class * implements com.google.gson.JsonDeserializer
