; Script generated by the Inno Setup Script Wizard.
; SEE THE DOCUMENTATION FOR DETAILS ON CREATING INNO SETUP SCRIPT FILES!

#define MyAppName "MiniProxy"
#define MyAppVersion "0.5"
#define MyAppExeName "proxy.exe"

[Setup]
; NOTE: The value of AppId uniquely identifies this application.
; Do not use the same AppId value in installers for other applications.
; (To generate a new GUID, click Tools | Generate GUID inside the IDE.)
AppId={{F066658A-A1E0-4AB2-9E56-4F9183D45F56}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
;AppVerName={#MyAppName} {#MyAppVersion}
DefaultDirName={pf}\{#MyAppName}
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
OutputDir=.\
OutputBaseFilename=setup
Compression=lzma
SolidCompression=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Files]
Source: "proxy.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "updater.exe"; DestDir: "{app}"
; NOTE: Don't use "Flags: ignoreversion" on any shared system files

[Run]
Filename: "{app}\{#MyAppExeName}"; Parameters: "install";
Filename: "{app}\{#MyAppExeName}"; Parameters: "start";
Filename: "SC"; Parameters: "failure MiniProxy reset= 432000  actions= restart/30000/restart/60000/restart/60000";

[UninstallRun]
Filename: "{app}\proxy.exe"; Parameters: "stop"
Filename: "{app}\proxy.exe"; Parameters: "remove"

[UninstallDelete]
Type: filesandordirs; Name: "{app}"
