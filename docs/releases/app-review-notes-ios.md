SCREEN RECORDING
A screen recording from a physical iPhone running the latest iOS is attached below. It starts at app launch and shows: adding the demo server, signing in, choosing a profile, browsing, playback with subtitles, Discover/Requests, downloads, and a watch party with chat, including reporting a message and blocking its sender.

1. PURPOSE AND AUDIENCE
Kinora is the iOS/tvOS client for Kinora, a free, open-source, self-hosted media server (similar to Jellyfin or Plex). It is for people who run their own Kinora server at home and want to watch their own movies and series on iPhone, iPad and Apple TV. The app contains no content of its own and sells nothing: it streams only the user's own files from the user's own server. Value: a native, ad-free player for a private library, with resume across devices, offline downloads, subtitles, parental profiles and watch parties for family and friends.

2. HOW TO ACCESS THE APP
1) Launch the app and tap "Add server" (Szerver hozzáadása).
2) Enter the demo server address: https://jellystream.projectone.dev/
3) Sign in with the user name and password in the Sign-In Information fields.
4) Pick a profile. Home shows the library; tap a title to play it.
Main features: Home and Libraries, Search, title details with ratings, playback with subtitle search, Downloads (offline), Discover and Requests (ask the server admin to add a title), Watch party (Party tab: create a room or join with a code/QR), Settings.
No sample files are needed: the demo account sees only a "Demo" library of Blender Foundation open movies (Big Buck Bunny, Elephants Dream, Sintel, Tears of Steel).

Accounts: there is no sign-up in the app. Accounts are created only by the owner (admin) of each private server, and are deleted by that admin on the server. The app does not create accounts, so no in-app account creation exists.

User-generated content: the only user content is the text chat inside a watch party. Rooms are private, joinable only with a code given by the host, and limited to accounts on the same private server. The chat is kept only in memory (last 50 messages) and is gone when the party ends.
- Report: press and hold any message from someone else > "Report". The message and its sender are stored on the server and shown to the server admin under Admin > Reports, who can remove the user's account.
- Block: press and hold a message (or a name in the member list) > "Block <name>". That person's messages are hidden for this user on this server, also in later parties; it can be undone from the member list.
On Apple TV the same actions are in the Party menu ("Report a message", "Block").

Payments: none. No in-app purchases, subscriptions or paid features.

3. EXTERNAL SERVICES
- The user's own Kinora server (all accounts, media and streaming).
- The server (not the app) fetches metadata from TMDB (The Movie Database API), ratings from OMDb, and subtitles from OpenSubtitles, using API keys configured by the server admin. TMDB is named as the source where its data is shown.
- Apple frameworks: AVKit/AVFoundation, SharePlay/GroupActivities, WidgetKit/ActivityKit, Bonjour (local server discovery).
No analytics, advertising, AI or payment services are used.

4. REGIONAL DIFFERENCES
The app works the same in all regions. The UI is in Hungarian, English and German. Only the "where to watch" information in Discover depends on the region set by the server admin.

5. THIRD-PARTY MATERIAL
The app does not provide or distribute any copyrighted media. Users play only files they own and store on their own server. Metadata and artwork come from TMDB under its API terms; ratings from OMDb and subtitles from OpenSubtitles under their API terms. The demo server's media are the Blender Foundation open movies, released under Creative Commons Attribution (CC BY), (c) Blender Foundation, blender.org.
