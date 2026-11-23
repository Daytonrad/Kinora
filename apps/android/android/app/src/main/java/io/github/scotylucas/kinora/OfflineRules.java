package io.github.scotylucas.kinora;

import java.util.ArrayList;
import java.util.List;
import java.util.Locale;

/**
 * The rules of offline downloads, without Android types (unit-tested on the JVM):
 * which server a page belongs to, which profile sees a download, file names.
 */
final class OfflineRules {
    private OfflineRules() {}

    /** "http://192.168.1.10:8096/x" → "192.168.1.10:8096" (lower case; the default port of the scheme is dropped). */
    static String hostOf(String url) {
        if (url == null) return "";
        String u = url.trim();
        int scheme = u.indexOf("://");
        if (scheme < 0) return "";
        String proto = u.substring(0, scheme).toLowerCase(Locale.ROOT);
        String rest = u.substring(scheme + 3);
        int end = rest.length();
        for (char c : new char[] {'/', '?', '#'}) {
            int i = rest.indexOf(c);
            if (i >= 0 && i < end) end = i;
        }
        String host = rest.substring(0, end).toLowerCase(Locale.ROOT);
        int at = host.lastIndexOf('@');
        if (at >= 0) host = host.substring(at + 1); // user:pass@host – never a Kinora address, but not a way around the check
        if (("http".equals(proto) && host.endsWith(":80")) || ("https".equals(proto) && host.endsWith(":443"))) {
            host = host.substring(0, host.lastIndexOf(':'));
        }
        return host;
    }

    /** The page is one of the server's known addresses (the launcher registered them). */
    static boolean pageIsServer(List<String> serverHosts, String pageUrl) {
        String h = hostOf(pageUrl);
        return !h.isEmpty() && serverHosts != null && serverHosts.contains(h);
    }

    /** A download URL may only point at the page's own server: absolute on the same host, or a path on it. */
    static String resolveOnPage(String pageUrl, String url) {
        if (url == null || url.isEmpty() || pageUrl == null) return null;
        int scheme = pageUrl.indexOf("://");
        if (scheme < 0) return null;
        String origin = pageUrl.substring(0, scheme + 3) + pageUrl.substring(scheme + 3).split("[/?#]", 2)[0];
        if (url.startsWith("/") && !url.startsWith("//")) return origin + url;
        if (!url.startsWith("http://") && !url.startsWith("https://")) return null;
        return hostOf(url).equals(hostOf(pageUrl)) ? url : null;
    }

    /**
     * Every profile sees only its own downloads (offline too, where no PIN or rating check reaches the server):
     * the server allowed the title to that profile when it was downloaded.
     */
    static boolean canSee(List<Integer> owners, long profileId) {
        return profileId > 0 && owners != null && owners.contains((int) profileId);
    }

    /** The owners after a profile deletes it; empty: the file itself can go. */
    static List<Integer> afterDelete(List<Integer> owners, long profileId) {
        List<Integer> rest = new ArrayList<>();
        if (owners != null) for (Integer o : owners) if (o != (int) profileId) rest.add(o);
        return rest;
    }

    /** A file name part from a server id: only [a-z0-9], at most 40 characters (no path can come from the page). */
    static String safePart(String s) {
        StringBuilder b = new StringBuilder();
        if (s != null) for (char c : s.toLowerCase(Locale.ROOT).toCharArray()) {
            if ((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) b.append(c);
            if (b.length() >= 40) break;
        }
        return b.length() == 0 ? "server" : b.toString();
    }
}
