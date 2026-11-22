import Foundation
import Testing
@testable import KinoraKit

struct SettingsTests {
    @Test func updateSendsOnlyChangedFields() throws {
        let data = try JSONEncoder.kinora.encode(ProfileUpdate(maxBitrate: 8000))
        let json = try #require(JSONSerialization.jsonObject(with: data) as? [String: Any])
        #expect(json.count == 1)
        #expect(json["max_bitrate"] as? Int == 8000)
    }

    @Test func createSendsServerKeys() throws {
        let data = try JSONEncoder.kinora.encode(ProfileCreate(name: "Kicsi", isKids: true))
        let json = try #require(JSONSerialization.jsonObject(with: data) as? [String: Any])
        #expect(json["name"] as? String == "Kicsi")
        #expect(json["is_kids"] as? Bool == true)
        #expect(ProfileCreate.colors.contains(json["color"] as? String ?? ""))
    }

    @Test func devicesDecode() throws {
        let body = #"{"devices":[{"id":"a","user_id":1,"name":"Nappali","platform":"webos","app_version":"1.2","ip":"192.168.1.5","created_at":1,"last_seen":100,"profile_id":null,"current":false}]}"#
        let list = try JSONDecoder.kinora.decode(DeviceList.self, from: Data(body.utf8)).devices
        #expect(list.first?.isTV == true)
        #expect(list.first?.platformLabel == "LG TV")
    }

    @Test func ago() {
        #expect(SettingChoices.ago(10) == "most")
        #expect(SettingChoices.ago(300) == "5 perce")
        #expect(SettingChoices.ago(3 * 3600) == "3 órája")
        #expect(SettingChoices.ago(3 * 86400) == "3 napja")
    }
}
