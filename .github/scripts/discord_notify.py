#!/usr/bin/env python3
"""GitHub release / issue események küldése Discord webhookokra."""
import json, os, re, sys, time, urllib.request, urllib.error, urllib.parse

LANGS = ["hu", "en", "de"]
DEEPL_CODE = {"hu": "HU", "en": "EN-GB", "de": "DE"}
T = {
    "hu": {"release": "Új kiadás", "pre": "előzetes", "more": "Letöltés és részletek", "cut": "A teljes változásnapló a GitHubon",
           "opened": "Új issue", "completed": "Megoldva", "not_planned": "Lezárva (nem lesz javítva)",
           "duplicate": "Lezárva (duplikátum)", "reopened": "Újranyitva", "labels": "Címkék", "test": "Tesztüzenet: a Kinora értesítések működnek ✅"},
    "en": {"release": "New release", "pre": "pre-release", "more": "Download & details", "cut": "Full changelog on GitHub",
           "opened": "New issue", "completed": "Solved", "not_planned": "Closed (won't fix)",
           "duplicate": "Closed (duplicate)", "reopened": "Reopened", "labels": "Labels", "test": "Test message: Kinora notifications work ✅"},
    "de": {"release": "Neues Release", "pre": "Vorabversion", "more": "Download & Details", "cut": "Vollständiges Changelog auf GitHub",
           "opened": "Neues Issue", "completed": "Gelöst", "not_planned": "Geschlossen (wird nicht behoben)",
           "duplicate": "Geschlossen (Duplikat)", "reopened": "Wieder geöffnet", "labels": "Labels", "test": "Testnachricht: Kinora-Benachrichtigungen funktionieren ✅"},
}
COLORS = {"release": 0x7C3AED, "opened": 0x2EA043, "completed": 0x8957E5, "not_planned": 0x6E7681,
          "duplicate": 0x6E7681, "reopened": 0xD29922}
EMOJI = {"opened": "🆕", "completed": "✅", "not_planned": "🚫", "duplicate": "🔁", "reopened": "🔄"}


def hook(name):
    return os.environ.get(name, "").strip()


def post(url, embed=None, content=None):
    payload = {"username": "Kinora", "allowed_mentions": {"parse": []}}
    if embed:
        payload["embeds"] = [embed]
    if content:
        payload["content"] = content
    data = json.dumps(payload).encode()
    for _ in range(5):
        req = urllib.request.Request(url + "?wait=true", data=data, method="POST",
                                     headers={"Content-Type": "application/json", "User-Agent": "kinora-notify"})
        try:
            with urllib.request.urlopen(req, timeout=30):
                return
        except urllib.error.HTTPError as e:
            raw = e.read().decode()
            if e.code == 429:
                time.sleep(float(json.loads(raw).get("retry_after", 1)) + 0.2)
                continue
            sys.exit(f"Discord HTTP {e.code}: {raw}")
    sys.exit("Discord rate limit, feladva.")


def section(body, lang):
    m = re.search(rf"<!--\s*{lang}\s*-->(.*?)<!--\s*/{lang}\s*-->", body, re.S | re.I)
    return m.group(1).strip() if m else None


def strip_sections(body):
    return re.sub(r"<!--\s*(\w{2})\s*-->.*?<!--\s*/\1\s*-->", "", body, flags=re.S | re.I).strip()


def deepl(text, lang):
    key = os.environ.get("DEEPL_API_KEY", "").strip()
    if not key or not text:
        return None
    host = "api-free.deepl.com" if key.endswith(":fx") else "api.deepl.com"
    data = urllib.parse.urlencode({"text": text, "target_lang": DEEPL_CODE[lang], "preserve_formatting": "1"}).encode()
    req = urllib.request.Request(f"https://{host}/v2/translate", data=data,
                                 headers={"Authorization": f"DeepL-Auth-Key {key}"})
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            return json.load(r)["translations"][0]["text"]
    except Exception as e:  # fordítás hiba esetén az eredeti megy ki
        print(f"DeepL hiba ({lang}): {e}", file=sys.stderr)
        return None


def discordify(md):
    md = re.sub(r"^#{1,6}\s*(.+)$", r"**\1**", md, flags=re.M)  # címsorok -> félkövér
    return re.sub(r"\n{3,}", "\n\n", md).strip()


def fit(text, limit, url, cut_label):
    if len(text) <= limit:
        return text
    return text[: limit - 80].rsplit("\n", 1)[0] + f"\n\n… [{cut_label}]({url})"


def on_release(ev):
    rel = ev["release"]
    body = rel.get("body") or ""
    common = strip_sections(body)
    name = rel.get("name") or rel["tag_name"]
    for lang in LANGS:
        url = hook(f"DISCORD_WEBHOOK_CHANGELOG_{lang.upper()}")
        if not url:
            continue
        t = T[lang]
        text = section(body, lang) or deepl(common, lang) or common
        desc = fit(discordify(text), 3800, rel["html_url"], t["cut"])
        desc += f"\n\n🔗 [{t['more']}]({rel['html_url']})"
        title = f"🚀 {t['release']}: {name}" + (f" ({t['pre']})" if rel.get("prerelease") else "")
        post(url, {"title": title[:256], "url": rel["html_url"], "description": desc,
                   "color": COLORS["release"], "timestamp": rel.get("published_at"),
                   "footer": {"text": rel["tag_name"]}})


def on_issue(ev):
    url = hook("DISCORD_WEBHOOK_ISSUES")
    if not url:
        return
    lang = os.environ.get("ISSUES_LANG", "en")
    t = T.get(lang, T["en"])
    issue, action = ev["issue"], ev["action"]
    kind = action if action in ("opened", "reopened") else (issue.get("state_reason") or "completed")
    kind = kind if kind in COLORS else "completed"
    embed = {
        "title": f"{EMOJI[kind]} {t[kind]} · #{issue['number']} {issue['title']}"[:256],
        "url": issue["html_url"],
        "color": COLORS[kind],
        "author": {"name": ev["sender"]["login"], "icon_url": ev["sender"]["avatar_url"],
                   "url": ev["sender"]["html_url"]},
        "timestamp": issue.get("closed_at") if action == "closed" else issue.get("updated_at"),
    }
    if action == "opened" and issue.get("body"):
        embed["description"] = fit(discordify(issue["body"]), 400, issue["html_url"], "…")
    labels = ", ".join(l["name"] for l in issue.get("labels", []))
    if labels:
        embed["fields"] = [{"name": t["labels"], "value": labels[:1024], "inline": True}]
    post(url, embed)


def on_test():
    for lang in LANGS:
        url = hook(f"DISCORD_WEBHOOK_CHANGELOG_{lang.upper()}")
        if url:
            post(url, content=T[lang]["test"])
    url = hook("DISCORD_WEBHOOK_ISSUES")
    if url:
        post(url, content=T.get(os.environ.get("ISSUES_LANG", "en"), T["en"])["test"])


def main():
    event = os.environ["GITHUB_EVENT_NAME"]
    with open(os.environ["GITHUB_EVENT_PATH"], encoding="utf-8") as f:
        ev = json.load(f)
    if event == "release":
        on_release(ev)
    elif event == "issues":
        on_issue(ev)
    elif event == "workflow_dispatch":
        on_test()


if __name__ == "__main__":
    main()
