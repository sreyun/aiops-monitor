package com.aiops.monitor

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Bundle
import android.os.Build
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.content.ContextCompat
import com.aiops.monitor.data.push.Notifications
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import com.aiops.monitor.data.store.SettingsStore
import com.aiops.monitor.data.store.ThemeMode
import com.aiops.monitor.ui.AIOpsApp
import com.aiops.monitor.ui.theme.AIOpsMonitorTheme
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow

class MainActivity : ComponentActivity() {
    private val notifPermLauncher = registerForActivityResult(
        ActivityResultContracts.RequestPermission()
    ) { }

    private val _deepLinkRoute = MutableSharedFlow<String>(extraBufferCapacity = 1)
    val deepLinkRoute: SharedFlow<String> = _deepLinkRoute.asSharedFlow()

    fun requestNotificationPermissionIfNeeded() {
        Notifications.ensureChannels(this)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) !=
            PackageManager.PERMISSION_GRANTED
        ) {
            notifPermLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
        }
    }

    private fun consumeDeepLink(intent: Intent?) {
        val route = intent?.getStringExtra(Notifications.EXTRA_NAV_ROUTE) ?: return
        if (route.isNotBlank()) _deepLinkRoute.tryEmit(route)
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge(
            statusBarStyle = SystemBarStyle.dark(android.graphics.Color.TRANSPARENT),
            navigationBarStyle = SystemBarStyle.dark(android.graphics.Color.rgb(15, 20, 25))
        )
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            window.isNavigationBarContrastEnforced = false
        }
        val settingsStore = SettingsStore(this)
        val initialDeepLink = intent?.getStringExtra(Notifications.EXTRA_NAV_ROUTE)
        setContent {
            val themeMode by settingsStore.themeMode.collectAsState(initial = ThemeMode.DARK)
            val darkTheme = themeMode == ThemeMode.DARK
            SideEffect {
                enableEdgeToEdge(
                    statusBarStyle = if (darkTheme) {
                        SystemBarStyle.dark(android.graphics.Color.TRANSPARENT)
                    } else {
                        SystemBarStyle.light(android.graphics.Color.TRANSPARENT, android.graphics.Color.TRANSPARENT)
                    },
                    navigationBarStyle = if (darkTheme) {
                        SystemBarStyle.dark(android.graphics.Color.rgb(12, 16, 24))
                    } else {
                        SystemBarStyle.light(android.graphics.Color.rgb(255, 255, 255), android.graphics.Color.rgb(255, 255, 255))
                    }
                )
            }
            AIOpsMonitorTheme(darkTheme = darkTheme) {
                Surface(
                    modifier = Modifier.fillMaxSize(),
                    color = MaterialTheme.colorScheme.background
                ) {
                    AIOpsApp(settingsStore = settingsStore, initialDeepLink = initialDeepLink)
                }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        consumeDeepLink(intent)
    }
}
