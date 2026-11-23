package io.github.scotylucas.kinora;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;

import java.util.Arrays;
import java.util.Collections;
import java.util.List;

import org.junit.Test;

public class OfflineRulesTest {
    @Test
    public void hostOf() {
        assertEquals("192.168.1.10:8096", OfflineRules.hostOf("http://192.168.1.10:8096/?app=android#/home"));
        assertEquals("kinora.example.org", OfflineRules.hostOf("https://Kinora.Example.org:443/x"));
        assertEquals("nas.local", OfflineRules.hostOf("http://nas.local:80"));
        assertEquals("evil.example", OfflineRules.hostOf("http://kinora.example.org@evil.example/"));
        assertEquals("", OfflineRules.hostOf("javascript:alert(1)"));
        assertEquals("", OfflineRules.hostOf(null));
    }

    @Test
    public void pageIsServer() {
        List<String> hosts = Arrays.asList("192.168.1.10:8096", "kinora.example.org");
        assertTrue(OfflineRules.pageIsServer(hosts, "http://192.168.1.10:8096/#/item/5"));
        assertTrue(OfflineRules.pageIsServer(hosts, "https://kinora.example.org/"));
        assertFalse(OfflineRules.pageIsServer(hosts, "http://192.168.1.11:8096/"));
        assertFalse(OfflineRules.pageIsServer(hosts, "http://localhost/"));
        assertFalse(OfflineRules.pageIsServer(null, "http://192.168.1.10:8096/"));
    }

    @Test
    public void downloadsOnlyFromThePagesServer() {
        String page = "http://192.168.1.10:8096/?app=android#/downloads";
        assertEquals("http://192.168.1.10:8096/api/downloads/k/file?st=x", OfflineRules.resolveOnPage(page, "/api/downloads/k/file?st=x"));
        assertEquals("http://192.168.1.10:8096/a", OfflineRules.resolveOnPage(page, "http://192.168.1.10:8096/a"));
        assertNull(OfflineRules.resolveOnPage(page, "http://other.example/a"));
        assertNull(OfflineRules.resolveOnPage(page, "//other.example/a"));
        assertNull(OfflineRules.resolveOnPage(page, "file:///data/data/x"));
        assertNull(OfflineRules.resolveOnPage(page, "content://x"));
    }

    @Test
    public void ownProfileOnly() {
        List<Integer> owners = Arrays.asList(1, 3);
        assertTrue(OfflineRules.canSee(owners, 1));
        assertTrue(OfflineRules.canSee(owners, 3));
        assertFalse(OfflineRules.canSee(owners, 2));
        assertFalse(OfflineRules.canSee(owners, 0));
        assertFalse(OfflineRules.canSee(null, 1));
    }

    @Test
    public void deleteKeepsTheFileForOthers() {
        assertEquals(Collections.singletonList(3), OfflineRules.afterDelete(Arrays.asList(1, 3), 1));
        assertTrue(OfflineRules.afterDelete(Collections.singletonList(1), 1).isEmpty());
        assertTrue(OfflineRules.afterDelete(null, 1).isEmpty());
    }

    @Test
    public void safePart() {
        assertEquals("acef0c867b386dd1", OfflineRules.safePart("acef0c867b386dd1"));
        assertEquals("etcpasswd", OfflineRules.safePart("../../etc/passwd"));
        assertEquals("server", OfflineRules.safePart(""));
        assertEquals(40, OfflineRules.safePart("a".repeat(100)).length());
    }
}
