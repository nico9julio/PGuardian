#ifndef AppVersion
  #define AppVersion "0.1.0"
#endif

[Setup]
AppId={{8D4F9B3A-7E21-4C8B-9F10-6A54D3E1C890}
AppName=PGuardian
AppVersion={#AppVersion}
AppPublisher=Nico (nico9julio)
AppPublisherURL=https://github.com/nico9julio/PGuardian
AppSupportURL=https://github.com/nico9julio/PGuardian/issues
AppUpdatesURL=https://github.com/nico9julio/PGuardian/releases
DefaultDirName={autopf}\PGuardian
DefaultGroupName=PGuardian
DisableProgramGroupPage=yes
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
SetupIconFile=assets\icon.ico
UninstallDisplayIcon={app}\PGuardian.exe
Compression=lzma2/fast
SolidCompression=yes
WizardStyle=modern
OutputDir=..\dist
OutputBaseFilename=PGuardianSetup_v{#AppVersion}
CloseApplications=force

[Languages]
Name: "spanish"; MessagesFile: "compiler:Languages\Spanish.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"

[Files]
Source: "..\dist\bin\PGuardian.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\dist\bin\PGuardianService.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\dist\bin\PGuardianTray.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\dist\bin\PGuardianUpdater.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "assets\icon.ico"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\PGuardian"; Filename: "{app}\PGuardian.exe"; IconFilename: "{app}\icon.ico"
Name: "{commondesktop}\PGuardian"; Filename: "{app}\PGuardian.exe"; IconFilename: "{app}\icon.ico"; Tasks: desktopicon

[Run]
; Create ProgramData\PGuardian and lock ACLs using language-neutral SIDs:
; S-1-5-18 = SYSTEM, S-1-5-32-544 = Built-in Administrators (protects DB/secrets/backups from standard RDP users)
Filename: "{cmd}"; Parameters: "/C mkdir ""{commonappdata}\PGuardian"" 2>nul & icacls ""{commonappdata}\PGuardian"" /inheritance:r /grant:r *S-1-5-18:(OI)(CI)F *S-1-5-32-544:(OI)(CI)F"; Flags: runhidden waituntilterminated
; Register and start Windows Service in Session 0
Filename: "{app}\PGuardianService.exe"; Parameters: "install"; Flags: runhidden waituntilterminated
Filename: "{sys}\sc.exe"; Parameters: "start PGuardianService"; Flags: runhidden waituntilterminated
; Launch native configurator / console after interactive installation
Filename: "{app}\PGuardian.exe"; Description: "Open PGuardian Configurator & Console"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM PGuardianTray.exe"; Flags: runhidden; RunOnceId: "KillTray"
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM PGuardian.exe"; Flags: runhidden; RunOnceId: "KillUI"
Filename: "{app}\PGuardianService.exe"; Parameters: "uninstall"; Flags: runhidden waituntilterminated; RunOnceId: "UninstallSvc"

[Code]
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  ResultCode: Integer;
begin
  // Gracefully stop running service & tray processes during in-place installer upgrades
  Exec(ExpandConstant('{sys}\sc.exe'), 'stop PGuardianService', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM PGuardianTray.exe', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM PGuardian.exe', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Sleep(800);
  Result := '';
end;
