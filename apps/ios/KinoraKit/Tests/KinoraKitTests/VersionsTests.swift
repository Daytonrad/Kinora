import Foundation
import Testing
@testable import KinoraKit

// The detail page's versions and the subtitle search, as the server sends them.
@Suite struct VersionsTests {
    @Test func decodesVersions() throws {
        let json = """
        {"item":{"id":7,"kind":"movie","title":"Mátrix","runtime_sec":8160,"user_data":{"position":120,"favorite":true}},
         "versions":[{"id":7,"resolution":"1080p","video_codec":"h264","audio_codec":"ac3","channels":6,"size":8000000000,"runtime_sec":8160},
                     {"id":9,"edition":"Director's Cut","resolution":"4K","hdr":"HDR10","size":40000000000,"position":3600}],
         "preferred_version":9}
        """
        let d = try JSONDecoder.kinora.decode(ItemDetail.self, from: Data(json.utf8))
        #expect(d.versions?.count == 2)
        #expect(d.preferredVersion == 9)
        let dc = try #require(d.versions?.last)
        #expect(dc.title == "Director's Cut · 4K")
        #expect(d.versions?.first?.details.hasPrefix("H264 · AC3 5.1") == true)
        let it = dc.item(from: d.item)
        #expect(it.id == 9)
        #expect(it.userData?.position == 3600)
        #expect(it.userData?.favorite == true)
    }

    @Test func decodesSubtitleSearch() throws {
        let json = """
        {"enabled":true,"langs":["hun","eng"],"results":[{"file_id":123,"lang":"hun","release":"Matrix.1999.1080p","downloads":40,"hash_match":true,"have":true}]}
        """
        let r = try JSONDecoder.kinora.decode(SubtitleSearchResult.self, from: Data(json.utf8))
        #expect(r.enabled)
        #expect(r.results.first?.fileId == 123)
        #expect(r.results.first?.hashMatch == true)
        #expect(r.results.first?.have == true)
        #expect(r.results.first?.name == "Matrix.1999.1080p")
    }
}
