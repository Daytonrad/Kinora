package io.github.scotylucas.kinora;

import android.app.Activity;
import android.webkit.JavascriptInterface;
import android.webkit.WebView;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;

/**
 * window.KinoraOffline: offline downloads (OfflineStore) for the launcher and the server's web client.
 *
 * Who may do what is decided from the page that is loaded, never from what the page claims:
 *  - the launcher (the app's own page, http://localhost) registers the servers it opens with their addresses,
 *    lists and plays the downloads of a server's last profile, and records what was watched offline;
 *  - a server's page is recognised by its address (one the launcher registered for that server). It reaches only
 *    that server's downloads, and may download only from its own address. Any other page gets an error.
 * Every method answers a JSON string: the result, or {"error": "..."}.
 */
public class KinoraOffline {
    private final Activity activity;
    private final WebView webView;
    private final OfflineStore store;

    KinoraOffline(Activity activity, WebView webView) {
        this.activity = activity;
        this.webView = webView;
        this.store = new OfflineStore(activity);
    }

    /** The top-level page's address (read on the UI thread; the bridge calls arrive on another one). */
    private String pageUrl() {
        final String[] url = new String[1];
        final CountDownLatch done = new CountDownLatch(1);
        activity.runOnUiThread(() -> {
            url[0] = webView.getUrl();
            done.countDown();
        });
        try {
            done.await(2, TimeUnit.SECONDS);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        }
        return url[0];
    }

    private boolean fromLauncher(String page) {
        return "localhost".equals(OfflineRules.hostOf(page));
    }

    private static String error(String message) {
        try {
            return new JSONObject().put("error", message).toString();
        } catch (Exception e) {
            return "{\"error\":\"error\"}";
        }
    }

    private interface Work {
        Object run() throws Exception;
    }

    private static String answer(Work w) {
        try {
            Object r = w.run();
            return r == null ? "{}" : r.toString();
        } catch (Exception e) {
            return error(e.getMessage() == null ? e.getClass().getSimpleName() : e.getMessage());
        }
    }

    @JavascriptInterface
    public boolean available() {
        return true;
    }

    // ------------------------------------------------------------------ the launcher

    /** Before it opens a server: its id and every address it may be reached at. */
    @JavascriptInterface
    public String registerServer(String id, String name, String urlsJson) {
        if (!fromLauncher(pageUrl())) return error("launcher only");
        return answer(() -> {
            store.registerServer(id, name, new JSONArray(urlsJson));
            return null;
        });
    }

    /** Servers whose last profile has downloads it may play. */
    @JavascriptInterface
    public String servers() {
        if (!fromLauncher(pageUrl())) return error("launcher only");
        return answer(store::offlineServers);
    }

    /** The downloads the server's last profile may play, with the queued position of each. */
    @JavascriptInterface
    public String saved(String server) {
        if (!fromLauncher(pageUrl())) return error("launcher only");
        return answer(() -> {
            JSONArray items = store.playable(server, store.lastProfile(server));
            for (int i = 0; i < items.length(); i++) {
                JSONObject q = store.queued(server, items.getJSONObject(i).optInt("id"));
                if (q != null) items.getJSONObject(i).put("queued", q);
            }
            return items;
        });
    }

    /** Watched offline: sent to the server by its web client next time. */
    @JavascriptInterface
    public String played(String server, int id, double position, boolean ended) {
        if (!fromLauncher(pageUrl())) return error("launcher only");
        return answer(() -> {
            store.played(server, id, position, ended);
            return null;
        });
    }

    /** Deleted offline by the last profile. */
    @JavascriptInterface
    public String forget(String server, int id) {
        if (!fromLauncher(pageUrl())) return error("launcher only");
        return answer(() -> {
            store.remove(server, id, store.lastProfile(server));
            return null;
        });
    }

    // ------------------------------------------------------------------ a server's web client

    private String serverOfPage(String page) throws Exception {
        String s = page == null || fromLauncher(page) ? null : store.serverForPage(page);
        if (s == null) throw new Exception("unknown server");
        return s;
    }

    /** The profile in use ({id, name, kids}); the launcher offers its downloads when the server is away. */
    @JavascriptInterface
    public String setProfile(String profileJson) {
        String page = pageUrl();
        return answer(() -> {
            store.setProfile(serverOfPage(page), new JSONObject(profileJson));
            return null;
        });
    }

    /** {id, key, title, meta, quality, profile}: the server prepares it; answers {"state": …}. */
    @JavascriptInterface
    public String want(String json) {
        String page = pageUrl();
        return answer(() -> new JSONObject().put("state", store.want(serverOfPage(page), new JSONObject(json))));
    }

    /** The server finished: download url (on this server only) and the subtitles [{lang, label, text}]. */
    @JavascriptInterface
    public String start(int id, String url, String subsJson) {
        String page = pageUrl();
        return answer(() -> {
            String server = serverOfPage(page);
            String abs = OfflineRules.resolveOnPage(page, url);
            if (abs == null) throw new Exception("the file must come from this server");
            store.start(server, id, abs, subsJson == null || subsJson.isEmpty() ? new JSONArray() : new JSONArray(subsJson));
            return null;
        });
    }

    /** This server's downloads on the device (with their owner profiles). */
    @JavascriptInterface
    public String list() {
        String page = pageUrl();
        return answer(() -> store.list(serverOfPage(page)));
    }

    @JavascriptInterface
    public String remove(int id, long profile) {
        String page = pageUrl();
        return answer(() -> {
            store.remove(serverOfPage(page), id, profile);
            return null;
        });
    }

    /** What the profile watched offline on this server's titles; taken off the queue. */
    @JavascriptInterface
    public String takeProgress(long profile) {
        String page = pageUrl();
        return answer(() -> store.takeProgress(serverOfPage(page), profile));
    }
}
