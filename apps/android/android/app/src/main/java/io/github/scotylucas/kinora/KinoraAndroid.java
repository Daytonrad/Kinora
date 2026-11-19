package io.github.scotylucas.kinora;

import android.app.Activity;
import android.app.UiModeManager;
import android.content.Context;
import android.content.pm.PackageManager;
import android.content.res.Configuration;
import android.os.Build;
import android.view.WindowManager;
import android.webkit.JavascriptInterface;

/**
 * window.KinoraAndroid: what the launcher and the server's web client (app mode, web/static/appshell.js)
 * need from the phone or TV. The server's pages are remote, so they get no Capacitor bridge; this small
 * synchronous interface is all they can reach, and nothing in it reads data or changes settings.
 */
public class KinoraAndroid {
    private final Activity activity;

    KinoraAndroid(Activity activity) {
        this.activity = activity;
    }

    static boolean isTv(Context ctx) {
        UiModeManager ui = (UiModeManager) ctx.getSystemService(Context.UI_MODE_SERVICE);
        return (ui != null && ui.getCurrentModeType() == Configuration.UI_MODE_TYPE_TELEVISION)
                || ctx.getPackageManager().hasSystemFeature(PackageManager.FEATURE_LEANBACK);
    }

    @JavascriptInterface
    public boolean isTv() {
        return isTv(activity);
    }

    @JavascriptInterface
    public String model() {
        return Build.MANUFACTURER + " " + Build.MODEL;
    }

    /** Keeps the screen on while a video plays. */
    @JavascriptInterface
    public void keepAwake(final boolean on) {
        activity.runOnUiThread(() -> {
            if (on) activity.getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
            else activity.getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        });
    }

    /** Back on the first screen: leave the app. */
    @JavascriptInterface
    public void exit() {
        activity.runOnUiThread(activity::finish);
    }
}
