package io.github.scotylucas.kinora;

import android.os.Bundle;
import android.webkit.WebView;

import androidx.activity.OnBackPressedCallback;

import com.getcapacitor.BridgeActivity;

public class MainActivity extends BridgeActivity {
    // window.kinoraBack() of the launcher and of the web client (appshell.js): true = handled
    // (a dialog or the player closed, one page back), false = first screen (leave the app),
    // anything else = a page without it (e.g. the WebView's error page): go back in the history.
    private static final String ASK_PAGE =
            "(function(){try{return window.kinoraBack?!!window.kinoraBack():null}catch(e){return null}})()";

    @Override
    public void onCreate(Bundle savedInstanceState) {
        registerPlugin(KinoraNativePlugin.class);
        super.onCreate(savedInstanceState);
        // added after Capacitor's own handler, so it runs first: Back (button, gesture, TV remote) asks the page
        getOnBackPressedDispatcher().addCallback(this, new OnBackPressedCallback(true) {
            @Override
            public void handleOnBackPressed() {
                WebView wv = getBridge().getWebView();
                wv.evaluateJavascript(ASK_PAGE, v -> {
                    if ("true".equals(v)) return;
                    if (!"false".equals(v) && wv.canGoBack()) wv.goBack();
                    else finish();
                });
            }
        });
    }
}
