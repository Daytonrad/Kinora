; Kinora – Windows installer (Inno Setup 6). Built by .github/workflows/windows.yml:
;   iscc /DVersion=1.2.3 /DSrc=..\..\dist\windows packaging\windows\kinora.iss
; Src must contain kinora.exe, kinora.ico, LICENSE and ffmpeg\ (ffmpeg.exe, ffprobe.exe, fpcalc.exe).
;
; Installs Kinora as a Windows service (kinora.exe service install …), opens the firewall for
; the chosen port (and UDP 7359 for app discovery), adds Start menu shortcuts and an uninstaller.
; Custom pages ask for the server name, port, logo and the first media folders; they are applied
; once, on the first start (afterwards the web admin is the place to change them).
; Updates run the installer silently (/VERYSILENT): the existing service and settings are kept.

#ifndef Version
  #define Version "0.0.0-dev"
#endif
#ifndef Src
  #define Src "..\..\dist\windows"
#endif

[Setup]
AppId={{6E1F4C8A-3B0D-4C55-9C7E-4B1A7D0F2E90}
AppName=Kinora
AppVersion={#Version}
AppVerName=Kinora {#Version}
AppPublisher=Kinora
AppPublisherURL=https://github.com/ScotyLucas/kinora
DefaultDirName={autopf}\Kinora
DefaultGroupName=Kinora
DisableProgramGroupPage=yes
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir=..\..\dist
OutputBaseFilename=KinoraSetup
SetupIconFile={#Src}\kinora.ico
UninstallDisplayIcon={app}\kinora.exe
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
CloseApplications=no
LicenseFile={#Src}\LICENSE

[Languages]
Name: "hu"; MessagesFile: "compiler:Languages\Hungarian.isl"
Name: "en"; MessagesFile: "compiler:Default.isl"

[Files]
Source: "{#Src}\kinora.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Src}\kinora.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Src}\LICENSE"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Src}\ffmpeg\*"; DestDir: "{app}\ffmpeg"; Flags: ignoreversion recursesubdirs

[Dirs]
Name: "{commonappdata}\Kinora"

[INI]
Filename: "{app}\Kinora.url"; Section: "InternetShortcut"; Key: "URL"; String: "http://localhost:{code:GetPort}"
Filename: "{app}\Kinora.url"; Section: "InternetShortcut"; Key: "IconFile"; String: "{app}\kinora.ico"
Filename: "{app}\Kinora.url"; Section: "InternetShortcut"; Key: "IconIndex"; String: "0"

[Icons]
Name: "{group}\Kinora megnyitása"; Filename: "{app}\Kinora.url"; IconFilename: "{app}\kinora.ico"
Name: "{group}\Kinora naplója"; Filename: "{commonappdata}\Kinora\kinora.log"
Name: "{group}\Kinora eltávolítása"; Filename: "{uninstallexe}"

[Registry]
Root: HKLM; Subkey: "Software\Kinora"; ValueType: string; ValueName: "Port"; ValueData: "{code:GetPort}"; Flags: uninsdeletekey
Root: HKLM; Subkey: "Software\Kinora"; ValueType: string; ValueName: "Version"; ValueData: "{#Version}"
Root: HKLM; Subkey: "Software\Kinora"; ValueType: string; ValueName: "InstallDir"; ValueData: "{app}"

[Run]
; first install: create the service with the wizard's answers (it starts immediately)
Filename: "{app}\kinora.exe"; Parameters: "{code:ServiceArgs}"; StatusMsg: "A Kinora szolgáltatás telepítése…"; Flags: runhidden waituntilterminated; Check: not IsUpgrade
; update: the service already exists with its settings, just start it again
Filename: "{app}\kinora.exe"; Parameters: "service start"; StatusMsg: "A Kinora újraindítása…"; Flags: runhidden waituntilterminated; Check: IsUpgrade
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall delete rule name=""Kinora"""; Flags: runhidden waituntilterminated
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall add rule name=""Kinora"" dir=in action=allow protocol=TCP localport={code:GetPort} program=""{app}\kinora.exe"" profile=private,domain"; StatusMsg: "Tűzfalszabály…"; Flags: runhidden waituntilterminated
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall add rule name=""Kinora"" dir=in action=allow protocol=UDP localport=7359 program=""{app}\kinora.exe"" profile=private,domain"; Flags: runhidden waituntilterminated
Filename: "{app}\Kinora.url"; Description: "A Kinora megnyitása a böngészőben"; Flags: postinstall shellexec nowait skipifsilent

[UninstallRun]
Filename: "{app}\kinora.exe"; Parameters: "service uninstall"; Flags: runhidden waituntilterminated; RunOnceId: "svc"
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall delete rule name=""Kinora"""; Flags: runhidden waituntilterminated; RunOnceId: "fw"

[Code]
var
  ServerPage: TInputQueryWizardPage;
  LogoPage: TInputFileWizardPage;
  MediaPage: TInputDirWizardPage;

function IsUpgrade(): Boolean;
begin
  Result := RegKeyExists(HKLM, 'SYSTEM\CurrentControlSet\Services\Kinora');
end;

function GetPort(Param: String): String;
begin
  // an update keeps the port chosen at the first install
  if IsUpgrade() and RegQueryStringValue(HKLM, 'Software\Kinora', 'Port', Result) and (Result <> '') then
    Exit;
  if (ServerPage <> nil) and (Trim(ServerPage.Values[1]) <> '') then
    Result := Trim(ServerPage.Values[1])
  else
    Result := '8096';
end;

function Quote(S: String): String;
begin
  Result := '"' + S + '"';
end;

function ServiceArgs(Param: String): String;
var
  Data, Media, Logo, Ext, Dst: String;
  I: Integer;
begin
  Data := ExpandConstant('{commonappdata}\Kinora');
  Result := 'service install --data ' + Quote(Data) + ' --addr :' + GetPort('');
  if Trim(ServerPage.Values[0]) <> '' then
    Result := Result + ' --initial-name ' + Quote(Trim(ServerPage.Values[0]));
  Logo := Trim(LogoPage.Values[0]);
  if (Logo <> '') and FileExists(Logo) then begin
    Ext := LowerCase(ExtractFileExt(Logo));
    Dst := Data + '\installer-logo' + Ext;
    if FileCopy(Logo, Dst, False) then
      Result := Result + ' --initial-logo ' + Quote(Dst);
  end;
  Media := '';
  for I := 0 to 2 do
    if Trim(MediaPage.Values[I]) <> '' then begin
      if Media <> '' then Media := Media + ';';
      Media := Media + Trim(MediaPage.Values[I]);
    end;
  if Media <> '' then
    Result := Result + ' --initial-media ' + Quote(Media);
end;

procedure InitializeWizard();
begin
  ServerPage := CreateInputQueryPage(wpSelectDir, 'A szerver', 'Hogyan hívják a szervert, és melyik porton fusson?',
    'A név a bejelentkezésnél és az appokban látszik. A port alapból 8096 (ezt kell beírni az appokba is).');
  ServerPage.Add('Szerver neve:', False);
  ServerPage.Add('Port:', False);
  ServerPage.Values[0] := GetComputerNameString();
  ServerPage.Values[1] := '8096';

  LogoPage := CreateInputFilePage(ServerPage.ID, 'Logó (nem kötelező)', 'Saját logó a szerverhez',
    'PNG, JPG, WebP vagy SVG, legfeljebb 2 MB. Üresen hagyva a Kinora logó látszik. Később az adminfelületen is cserélhető.');
  LogoPage.Add('Logófájl:', 'Képek|*.png;*.jpg;*.jpeg;*.webp;*.svg|Minden fájl|*.*', '.png');

  MediaPage := CreateInputDirPage(LogoPage.ID, 'Médiamappák', 'Hol vannak a filmjeid és sorozataid?',
    'Ezeket a mappákat a Kinora az első indításkor könyvtárként felveszi. Később az Admin → Könyvtárak oldalon bármikor adhatsz hozzá újat.',
    False, '');
  MediaPage.Add('1. mappa (pl. Filmek):');
  MediaPage.Add('2. mappa (pl. Sorozatok):');
  MediaPage.Add('3. mappa:');
end;

function ShouldSkipPage(PageID: Integer): Boolean;
begin
  // an update keeps the existing service, its port and its settings
  Result := IsUpgrade() and ((PageID = ServerPage.ID) or (PageID = LogoPage.ID) or (PageID = MediaPage.ID));
end;

function NextButtonClick(CurPageID: Integer): Boolean;
var
  P: Integer;
begin
  Result := True;
  if CurPageID = ServerPage.ID then begin
    P := StrToIntDef(Trim(ServerPage.Values[1]), 0);
    if (P < 1) or (P > 65535) then begin
      MsgBox('A port 1 és 65535 közötti szám legyen.', mbError, MB_OK);
      Result := False;
    end;
  end;
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  Code: Integer;
begin
  // stop the running service so its files can be replaced
  if IsUpgrade() then
    Exec(ExpandConstant('{sys}\sc.exe'), 'stop Kinora', '', SW_HIDE, ewWaitUntilTerminated, Code);
  Sleep(1500);
  Result := '';
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
    if MsgBox('Törlöd a Kinora adatait is (adatbázis, képek, beállítások: ' + ExpandConstant('{commonappdata}\Kinora') + ')?' + #13#10 +
              'Ez nem vonható vissza. Nem esetén egy újratelepítés ott folytatja, ahol abbahagytad.',
              mbConfirmation, MB_YESNO or MB_DEFBUTTON2) = IDYES then
      DelTree(ExpandConstant('{commonappdata}\Kinora'), True, True, True);
end;
