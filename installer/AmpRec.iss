#ifndef AppVersion
  #define AppVersion "1.1.0"
#endif

#define AppName "AmpRec"
#define AppPublisher "Ablaka Team"
#define AppURL "https://github.com/ablakateam/AmpRec"

[Setup]
AppId={{6D58C0C5-642C-4984-BDFE-20560CC1DFD8}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}/issues
AppUpdatesURL={#AppURL}/releases/latest
DefaultDirName={localappdata}\Programs\AmpRec
DefaultGroupName=AmpRec
AllowNoIcons=yes
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0.20348
WizardStyle=modern
WizardSizePercent=100
Compression=lzma2/ultra64
SolidCompression=yes
OutputDir=..\dist
OutputBaseFilename=AmpRec-Setup-{#AppVersion}-win-x64
SetupIconFile=..\assets\AmpRec.ico
UninstallDisplayIcon={app}\AmpRec.exe
LicenseFile=..\LICENSE
CloseApplications=yes
CloseApplicationsFilter=AmpRec.exe
RestartApplications=no
VersionInfoVersion={#AppVersion}.0
VersionInfoCompany={#AppPublisher}
VersionInfoDescription=AmpRec Windows Installer
VersionInfoProductName={#AppName}
VersionInfoProductVersion={#AppVersion}

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "Create a &desktop shortcut"; GroupDescription: "Additional shortcuts:"; Flags: unchecked

[Files]
Source: "..\dist\AmpRec.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\dist\README.txt"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\AmpRec"; Filename: "{app}\AmpRec.exe"; WorkingDir: "{app}"
Name: "{group}\Uninstall AmpRec"; Filename: "{uninstallexe}"
Name: "{autodesktop}\AmpRec"; Filename: "{app}\AmpRec.exe"; WorkingDir: "{app}"; Tasks: desktopicon

[Run]
Filename: "{app}\AmpRec.exe"; Description: "Launch AmpRec"; Flags: nowait postinstall skipifsilent unchecked
