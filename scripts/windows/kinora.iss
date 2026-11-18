; Kinora Windows telepítő (Inno Setup 6). A CI építi: ISCC /DAppVersion=1.2.3 /DSourceDir=dist\kinora kinora.iss
; A fájlokat a {app}-ba másolja, majd az install.ps1 -Configure elvégzi a többit:
; Windows-szolgáltatás, tűzfal, napi automatikus frissítés (update.ps1). Az adatok (C:\ProgramData\Kinora) megmaradnak.

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef SourceDir
  #define SourceDir "..\..\dist\kinora"
#endif

[Setup]
AppId={{6E0F3C2A-8B4D-4E1F-9A7C-2D5B8E1F4A90}
AppName=Kinora
AppVersion={#AppVersion}
AppVerName=Kinora {#AppVersion}
AppPublisher=Kinora
AppPublisherURL=https://github.com/ScotyLucas/kinora
DefaultDirName={autopf}\Kinora
DisableProgramGroupPage=yes
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir=.
OutputBaseFilename=kinora-setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
UninstallDisplayIcon={app}\kinora.exe
LicenseFile={#SourceDir}\LICENSE
CloseApplications=no

[Languages]
Name: "hu"; MessagesFile: "compiler:Languages\Hungarian.isl"
Name: "en"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "autoupdate"; Description: "Automatikus frissítés naponta (csak kiadott verziókra)"

[Files]
Source: "{#SourceDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Run]
Filename: "powershell.exe"; Parameters: "{code:ConfigureArgs}"; Flags: runhidden waituntilterminated; StatusMsg: "Szolgáltatás, tűzfal és frissítés beállítása…"
Filename: "http://localhost:{code:GetPort}"; Description: "A Kinora megnyitása a böngészőben"; Flags: postinstall shellexec nowait skipifsilent

[UninstallRun]
Filename: "{app}\kinora.exe"; Parameters: "service uninstall"; Flags: runhidden waituntilterminated; RunOnceId: "svc"
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -Command ""Unregister-ScheduledTask -TaskName 'Kinora frissítés' -Confirm:$false -ErrorAction SilentlyContinue; Get-NetFirewallRule -DisplayName 'Kinora*' -ErrorAction SilentlyContinue | Remove-NetFirewallRule"""; Flags: runhidden waituntilterminated; RunOnceId: "cleanup"

[UninstallDelete]
Type: filesandordirs; Name: "{app}\backup"
Type: files; Name: "{app}\update.log"

[Code]
var
  SettingsPage: TInputQueryWizardPage;

procedure InitializeWizard;
begin
  SettingsPage := CreateInputQueryPage(wpSelectTasks,
    'Beállítások', 'Port és médiamappák',
    'A médiamappákat csak az első indításkor veszi fel; később a webes felületen módosíthatod őket.' + #13#10 +
    'Formátum: Név=útvonal, pontosvesszővel elválasztva, pl. Filmek=D:\Filmek;Sorozatok=D:\Sorozatok' + #13#10 +
    'Hálózati megosztásnál UNC útvonalat adj meg (\\nas\Filmek), ne meghajtóbetűt.');
  SettingsPage.Add('Port:', False);
  SettingsPage.Add('Médiamappák (elhagyható):', False);
  SettingsPage.Values[0] := GetPreviousData('Port', '8096');
  SettingsPage.Values[1] := GetPreviousData('Media', '');
end;

procedure RegisterPreviousData(PreviousDataKey: Integer);
begin
  SetPreviousData(PreviousDataKey, 'Port', SettingsPage.Values[0]);
  SetPreviousData(PreviousDataKey, 'Media', SettingsPage.Values[1]);
end;

function NextButtonClick(CurPageID: Integer): Boolean;
var
  P: Integer;
begin
  Result := True;
  if CurPageID = SettingsPage.ID then
  begin
    P := StrToIntDef(Trim(SettingsPage.Values[0]), 0);
    if (P < 1) or (P > 65535) then
    begin
      MsgBox('A port 1 és 65535 közötti szám legyen.', mbError, MB_OK);
      Result := False;
    end;
    if Pos('"', SettingsPage.Values[1]) > 0 then
    begin
      MsgBox('A médiamappák nem tartalmazhatnak idézőjelet.', mbError, MB_OK);
      Result := False;
    end;
  end;
end;

function GetPort(Param: String): String;
begin
  Result := Trim(SettingsPage.Values[0]);
end;

function ConfigureArgs(Param: String): String;
begin
  Result := '-NoProfile -ExecutionPolicy Bypass -File "' + ExpandConstant('{app}\install.ps1') + '" -Configure' +
    ' -InstallDir "' + ExpandConstant('{app}') + '" -Port ' + GetPort('');
  if Trim(SettingsPage.Values[1]) <> '' then
    Result := Result + ' -Media "' + Trim(SettingsPage.Values[1]) + '"';
  if not WizardIsTaskSelected('autoupdate') then
    Result := Result + ' -NoAutoUpdate';
end;

// frissítéskor a futó szolgáltatás zárolja a kinora.exe-t: másolás előtt leállítjuk
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  Code: Integer;
begin
  Result := '';
  Exec(ExpandConstant('{sys}\net.exe'), 'stop Kinora', '', SW_HIDE, ewWaitUntilTerminated, Code);
  // a szolgáltatás leállta után maradt ffmpeg-folyamatok is zárolhatják a fájlokat
  Exec('powershell.exe', '-NoProfile -Command "Get-Process ffmpeg,ffprobe -ErrorAction SilentlyContinue | ' +
    'Where-Object { $_.Path -like ''' + ExpandConstant('{app}') + '\*'' } | Stop-Process -Force"',
    '', SW_HIDE, ewWaitUntilTerminated, Code);
end;
