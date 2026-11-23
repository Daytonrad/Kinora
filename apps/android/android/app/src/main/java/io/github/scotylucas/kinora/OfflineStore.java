package io.github.scotylucas.kinora;

import android.app.DownloadManager;
import android.content.Context;
import android.database.Cursor;
import android.net.Uri;
import android.os.Environment;

import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;

import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Iterator;
import java.util.List;

/**
 * Offline downloads of the Android app. The server prepares an MP4 (internal/media/download.go), the system's
 * DownloadManager fetches it into the app's own folder (in the background, with a notification, across
 * restarts), and the launcher plays it when the server can't be reached.
 *
 * index.json in the app's private files:
 *   servers:  id → {name, hosts[], lastProfile{id,name,kids}}   (hosts: the addresses the launcher opened it on)
 *   items:    [{server, id, title, meta{}, quality, key, state, dm, file, subs[], profiles[], size, savedAt}]
 *             state: preparing (the server transcodes) | downloading | done | failed
 *   progress: "server|id" → {position, ended}   (watched offline, sent by the web client once online)
 */
final class OfflineStore {
    private static final int MAX_SUBS = 30;
    private static final int MAX_SUB_BYTES = 2 * 1024 * 1024;

    private final Context ctx;
    private final File dir;
    private JSONObject index;

    OfflineStore(Context ctx) {
        this.ctx = ctx.getApplicationContext();
        dir = new File(this.ctx.getFilesDir(), "offline");
        //noinspection ResultOfMethodCallIgnored
        dir.mkdirs();
        load();
    }

    // ------------------------------------------------------------------ servers and profiles

    synchronized void registerServer(String id, String name, JSONArray urls) throws JSONException {
        JSONObject servers = index.getJSONObject("servers");
        JSONObject s = servers.optJSONObject(id);
        if (s == null) s = new JSONObject();
        JSONArray hosts = new JSONArray();
        for (int i = 0; i < urls.length(); i++) {
            String h = OfflineRules.hostOf(urls.optString(i));
            if (!h.isEmpty() && !"localhost".equals(h)) hosts.put(h);
        }
        s.put("name", name == null ? "" : name);
        s.put("hosts", hosts);
        servers.put(id, s);
        save();
    }

    /** The server whose page this is, or null (the launcher, or an address the launcher never opened). */
    synchronized String serverForPage(String pageUrl) {
        JSONObject servers = index.optJSONObject("servers");
        if (servers == null) return null;
        for (Iterator<String> it = servers.keys(); it.hasNext(); ) {
            String id = it.next();
            JSONArray hosts = servers.optJSONObject(id).optJSONArray("hosts");
            if (OfflineRules.pageIsServer(strings(hosts), pageUrl)) return id;
        }
        return null;
    }

    /** The profile in use on the server's pages; the launcher shows its downloads when the server is away. */
    synchronized void setProfile(String server, JSONObject profile) throws JSONException {
        JSONObject s = index.getJSONObject("servers").optJSONObject(server);
        if (s == null) return;
        JSONObject p = new JSONObject();
        p.put("id", profile.optLong("id"));
        p.put("name", profile.optString("name"));
        p.put("kids", profile.optBoolean("kids"));
        s.put("lastProfile", p);
        save();
    }

    synchronized long lastProfile(String server) {
        JSONObject s = index.optJSONObject("servers").optJSONObject(server);
        JSONObject p = s == null ? null : s.optJSONObject("lastProfile");
        return p == null ? 0 : p.optLong("id");
    }

    /** For the launcher: servers that have downloads their last profile may play. */
    synchronized JSONArray offlineServers() throws JSONException {
        refresh();
        JSONArray out = new JSONArray();
        JSONObject servers = index.getJSONObject("servers");
        for (Iterator<String> it = servers.keys(); it.hasNext(); ) {
            String id = it.next();
            JSONObject s = servers.getJSONObject(id);
            JSONObject p = s.optJSONObject("lastProfile");
            if (p == null) continue;
            int n = 0;
            for (JSONObject item : items(id)) {
                if ("done".equals(item.optString("state")) && OfflineRules.canSee(ints(item.optJSONArray("profiles")), p.optLong("id"))) n++;
            }
            if (n == 0) continue;
            out.put(new JSONObject().put("id", id).put("name", s.optString("name")).put("profile", p).put("count", n));
        }
        return out;
    }

    // ------------------------------------------------------------------ downloads

    /**
     * The web client asked the server to prepare a title for this profile. A title another profile of this
     * server already saved is not fetched again: it just becomes this profile's too ("done").
     */
    synchronized String want(String server, JSONObject in) throws JSONException {
        int id = in.getInt("id");
        long profile = in.optLong("profile");
        if (profile <= 0) throw new JSONException("no profile");
        JSONObject item = find(server, id);
        if (item == null) {
            item = new JSONObject().put("server", server).put("id", id).put("profiles", new JSONArray()).put("state", "preparing");
            index.getJSONArray("items").put(item);
        }
        JSONArray owners = item.getJSONArray("profiles");
        if (!ints(owners).contains((int) profile)) owners.put(profile);
        if ("done".equals(item.optString("state")) && new File(item.optString("file")).isFile()) {
            save();
            return "done";
        }
        item.put("title", in.optString("title"));
        item.put("meta", in.optJSONObject("meta") == null ? new JSONObject() : in.getJSONObject("meta"));
        item.put("quality", in.optString("quality"));
        item.put("key", in.optString("key"));
        if (!"downloading".equals(item.optString("state"))) item.put("state", "preparing");
        item.put("error", "");
        save();
        return item.getString("state");
    }

    /** The server finished preparing: the DownloadManager takes over. url is already checked to be on the server. */
    synchronized void start(String server, int id, String url, JSONArray subs) throws JSONException, IOException {
        JSONObject item = find(server, id);
        if (item == null) throw new JSONException("not requested");
        if ("downloading".equals(item.optString("state")) || "done".equals(item.optString("state"))) return;
        File movies = ctx.getExternalFilesDir(Environment.DIRECTORY_MOVIES);
        if (movies == null) throw new IOException("no storage");
        String name = OfflineRules.safePart(server) + "-" + id;

        // subtitles: written now (the web client fetched them with its session), at most 30, 2 MB each
        JSONArray saved = new JSONArray();
        for (int i = 0; subs != null && i < subs.length() && i < MAX_SUBS; i++) {
            JSONObject s = subs.getJSONObject(i);
            byte[] text = s.optString("text").getBytes(StandardCharsets.UTF_8);
            if (text.length == 0 || text.length > MAX_SUB_BYTES) continue;
            File f = new File(dir, name + "." + i + ".vtt");
            try (FileOutputStream out = new FileOutputStream(f)) { out.write(text); }
            saved.put(new JSONObject().put("lang", s.optString("lang")).put("label", s.optString("label")).put("file", f.getAbsolutePath()));
        }

        File dest = new File(movies, "kinora/" + name + ".mp4");
        //noinspection ResultOfMethodCallIgnored
        dest.getParentFile().mkdirs();
        if (dest.exists()) //noinspection ResultOfMethodCallIgnored
            dest.delete();
        DownloadManager dm = (DownloadManager) ctx.getSystemService(Context.DOWNLOAD_SERVICE);
        DownloadManager.Request req = new DownloadManager.Request(Uri.parse(url))
                .setTitle(item.optString("title", "Kinora"))
                .setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED)
                .setDestinationInExternalFilesDir(ctx, Environment.DIRECTORY_MOVIES, "kinora/" + name + ".mp4");
        long dmId = dm.enqueue(req);
        item.put("dm", dmId).put("file", dest.getAbsolutePath()).put("subs", saved).put("state", "downloading").put("error", "");
        save();
    }

    /** Takes the DownloadManager's word for the running transfers. */
    private void refresh() throws JSONException {
        DownloadManager dm = (DownloadManager) ctx.getSystemService(Context.DOWNLOAD_SERVICE);
        boolean changed = false;
        for (JSONObject item : items(null)) {
            if (!"downloading".equals(item.optString("state"))) continue;
            long dmId = item.optLong("dm", -1);
            int status = -1;
            long done = 0, total = 0;
            int reason = 0;
            try (Cursor c = dm.query(new DownloadManager.Query().setFilterById(dmId))) {
                if (c != null && c.moveToFirst()) {
                    status = c.getInt(c.getColumnIndexOrThrow(DownloadManager.COLUMN_STATUS));
                    done = c.getLong(c.getColumnIndexOrThrow(DownloadManager.COLUMN_BYTES_DOWNLOADED_SO_FAR));
                    total = c.getLong(c.getColumnIndexOrThrow(DownloadManager.COLUMN_TOTAL_SIZE_BYTES));
                    reason = c.getInt(c.getColumnIndexOrThrow(DownloadManager.COLUMN_REASON));
                }
            }
            if (status == DownloadManager.STATUS_SUCCESSFUL) {
                item.put("state", "done").put("size", new File(item.optString("file")).length()).put("savedAt", System.currentTimeMillis());
                changed = true;
            } else if (status == DownloadManager.STATUS_FAILED || status == -1) {
                item.put("state", "failed").put("error", status == -1 ? "cancelled" : "error " + reason);
                changed = true;
            } else {
                item.put("progress", total > 0 ? (double) done / total : 0); // shown, not worth a write
            }
        }
        if (changed) save();
    }

    /** The server's downloads (every profile's: the web client shows the current profile's). */
    synchronized JSONArray list(String server) throws JSONException {
        refresh();
        JSONArray out = new JSONArray();
        for (JSONObject item : items(server)) out.put(item);
        return out;
    }

    /** For the launcher: the done downloads the profile may play. */
    synchronized JSONArray playable(String server, long profile) throws JSONException {
        refresh();
        JSONArray out = new JSONArray();
        for (JSONObject item : items(server)) {
            if ("done".equals(item.optString("state")) && OfflineRules.canSee(ints(item.optJSONArray("profiles")), profile)
                    && new File(item.optString("file")).isFile()) out.put(item);
        }
        return out;
    }

    /** A profile lets go of a title; the file goes when no profile has it any more. */
    synchronized void remove(String server, int id, long profile) throws JSONException {
        JSONObject item = find(server, id);
        if (item == null) return;
        List<Integer> rest = OfflineRules.afterDelete(ints(item.optJSONArray("profiles")), profile);
        if (!rest.isEmpty()) {
            JSONArray owners = new JSONArray();
            for (Integer o : rest) owners.put(o);
            item.put("profiles", owners);
            save();
            return;
        }
        long dmId = item.optLong("dm", -1);
        if (dmId >= 0) ((DownloadManager) ctx.getSystemService(Context.DOWNLOAD_SERVICE)).remove(dmId); // also deletes the file
        //noinspection ResultOfMethodCallIgnored
        new File(item.optString("file", "/nonexistent")).delete();
        JSONArray subs = item.optJSONArray("subs");
        for (int i = 0; subs != null && i < subs.length(); i++) //noinspection ResultOfMethodCallIgnored
            new File(subs.getJSONObject(i).optString("file", "/nonexistent")).delete();
        JSONArray all = index.getJSONArray("items");
        JSONArray keep = new JSONArray();
        for (int i = 0; i < all.length(); i++) if (all.getJSONObject(i) != item) keep.put(all.getJSONObject(i));
        index.put("items", keep);
        save();
    }

    // ------------------------------------------------------------------ watched offline

    synchronized void played(String server, int id, double position, boolean ended) throws JSONException {
        JSONObject q = index.getJSONObject("progress");
        String k = server + "|" + id;
        JSONObject was = q.optJSONObject(k);
        // a finished viewing stays finished even if a later report says otherwise
        if (was != null && was.optBoolean("ended") && !ended && position < 1) return;
        q.put(k, new JSONObject().put("server", server).put("id", id).put("position", position).put("ended", ended)
                .put("profile", lastProfile(server))); // watched offline: always the server's last profile
        save();
    }

    /** The profile's queued positions on this server, removed from the queue (the web client sends them). */
    synchronized JSONArray takeProgress(String server, long profile) throws JSONException {
        JSONObject q = index.getJSONObject("progress");
        JSONArray out = new JSONArray();
        List<String> sent = new ArrayList<>();
        for (Iterator<String> it = q.keys(); it.hasNext(); ) {
            String k = it.next();
            JSONObject e = q.getJSONObject(k);
            if (server.equals(e.optString("server")) && e.optLong("profile") == profile) { out.put(e); sent.add(k); }
        }
        for (String k : sent) q.remove(k);
        if (!sent.isEmpty()) save();
        return out;
    }

    synchronized JSONObject queued(String server, int id) {
        return index.optJSONObject("progress").optJSONObject(server + "|" + id);
    }

    // ------------------------------------------------------------------ storage

    private JSONObject find(String server, int id) throws JSONException {
        for (JSONObject item : items(server)) if (item.optInt("id") == id) return item;
        return null;
    }

    private List<JSONObject> items(String server) throws JSONException {
        List<JSONObject> out = new ArrayList<>();
        JSONArray all = index.getJSONArray("items");
        for (int i = 0; i < all.length(); i++) {
            JSONObject o = all.getJSONObject(i);
            if (server == null || server.equals(o.optString("server"))) out.add(o);
        }
        return out;
    }

    private static List<String> strings(JSONArray a) {
        List<String> out = new ArrayList<>();
        for (int i = 0; a != null && i < a.length(); i++) out.add(a.optString(i));
        return out;
    }

    private static List<Integer> ints(JSONArray a) {
        List<Integer> out = new ArrayList<>();
        for (int i = 0; a != null && i < a.length(); i++) out.add(a.optInt(i));
        return out;
    }

    private void load() {
        File f = new File(dir, "index.json");
        try (FileInputStream in = new FileInputStream(f)) { // minSdk 24: no java.nio.file
            ByteArrayOutputStream buf = new ByteArrayOutputStream();
            byte[] b = new byte[16384];
            for (int n; (n = in.read(b)) > 0; ) buf.write(b, 0, n);
            index = new JSONObject(new String(buf.toByteArray(), StandardCharsets.UTF_8));
        } catch (Exception e) {
            index = new JSONObject();
        }
        try {
            if (!index.has("servers")) index.put("servers", new JSONObject());
            if (!index.has("items")) index.put("items", new JSONArray());
            if (!index.has("progress")) index.put("progress", new JSONObject());
        } catch (JSONException ignored) { /* cannot happen */ }
    }

    private void save() {
        File tmp = new File(dir, "index.json.tmp");
        try (FileOutputStream out = new FileOutputStream(tmp)) {
            out.write(index.toString().getBytes(StandardCharsets.UTF_8));
            out.getFD().sync();
        } catch (IOException e) {
            return;
        }
        //noinspection ResultOfMethodCallIgnored
        tmp.renameTo(new File(dir, "index.json"));
    }
}
