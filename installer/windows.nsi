Unicode true
ManifestDPIAware true
RequestExecutionLevel admin
SetCompressor /SOLID lzma

!include MUI2.nsh
!include LogicLib.nsh
!include x64.nsh

!define APP "Discord DNS"
!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\discord-dns"
!define LOGFILE "C:\ProgramData\discord-dns\discord-dns.log"

Name "${APP}"
OutFile "${OUTFILE}"
InstallDir "$PROGRAMFILES64\discord-dns"
BrandingText "${APP} ${VERSION}"
ShowInstDetails show
ShowUninstDetails show

!define MUI_ICON "${ICON}"
!define MUI_UNICON "${ICON}"
!define MUI_ABORTWARNING
!define MUI_WELCOMEPAGE_TITLE "${APP} Kurulumu"
!define MUI_WELCOMEPAGE_TEXT "Bu sihirbaz ${APP} ${VERSION} sürümünü kuracak.$\r$\n$\r$\n${APP} sadece Discord trafiğini engelden geçirir; diğer internet trafiğine dokunmaz. Arka planda çalışır ve bilgisayar her açıldığında kendiliğinden başlar.$\r$\n$\r$\nDaha önce kurduysan bu sihirbaz programı günceller."
!define MUI_FINISHPAGE_TITLE "Kurulum tamamlandı"
!define MUI_FINISHPAGE_TEXT "${APP} arka planda çalışıyor.$\r$\n$\r$\nDiscord açıksa tamamen kapatıp yeniden aç.$\r$\n$\r$\nKaldırmak için: Ayarlar → Uygulamalar → ${APP}."
!define MUI_UNCONFIRMPAGE_TEXT_TOP "${APP} kaldırılacak ve DNS ayarların kurulum öncesi haline döndürülecek."

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${LICENSE}"
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "Turkish"

VIProductVersion "${WINVER4}"
VIAddVersionKey /LANG=${LANG_TURKISH} "ProductName" "${APP}"
VIAddVersionKey /LANG=${LANG_TURKISH} "FileDescription" "${APP} Kurulum"
VIAddVersionKey /LANG=${LANG_TURKISH} "ProductVersion" "${VERSION}"
VIAddVersionKey /LANG=${LANG_TURKISH} "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=${LANG_TURKISH} "CompanyName" "SalihUstun"
VIAddVersionKey /LANG=${LANG_TURKISH} "LegalCopyright" "Copyright (c) 2026 SalihUstun. MIT License."

Function .onInit
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP "${APP} 64 bit Windows gerektirir."
    Abort
  ${EndIf}
  SetRegView 64
FunctionEnd

Section "Kur"
  InitPluginsDir
  SetOutPath "$PLUGINSDIR\app"
  ${If} ${IsNativeARM64}
    File /oname=discord-dns.exe "${EXE_ARM64}"
  ${Else}
    File /oname=discord-dns.exe "${EXE_X64}"
  ${EndIf}

  DetailPrint "Servis kuruluyor ve başlatılıyor..."
  nsExec::Exec '"$PLUGINSDIR\app\discord-dns.exe" install'
  Pop $0
  ${If} $0 != 0
    DetailPrint "Kurulum başarısız (hata kodu: $0)."
    MessageBox MB_ICONSTOP "Kurulum tamamlanamadı (hata kodu: $0).$\r$\n$\r$\nAyrıntılar: ${LOGFILE}"
    Abort
  ${EndIf}
  DetailPrint "Servis çalışıyor, sistem DNS'i ayarlandı."

  SetOutPath "$INSTDIR"
  WriteUninstaller "$INSTDIR\uninstall.exe"
  WriteRegStr HKLM "${UNINST_KEY}" "DisplayName" "${APP}"
  WriteRegStr HKLM "${UNINST_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "${UNINST_KEY}" "Publisher" "SalihUstun"
  WriteRegStr HKLM "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\discord-dns.exe"
  WriteRegStr HKLM "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "${UNINST_KEY}" "URLInfoAbout" "https://github.com/SalihUstun/discord-dns"
  WriteRegStr HKLM "${UNINST_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKLM "${UNINST_KEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegDWORD HKLM "${UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKLM "${UNINST_KEY}" "NoRepair" 1
  WriteRegDWORD HKLM "${UNINST_KEY}" "EstimatedSize" 16000
  SetOutPath "$TEMP"
SectionEnd

Function un.onInit
  SetRegView 64
FunctionEnd

Section "Uninstall"
  DetailPrint "Servis durduruluyor, DNS ayarları geri yükleniyor..."
  nsExec::Exec '"$INSTDIR\discord-dns.exe" uninstall'
  Pop $0
  ${If} $0 != 0
    DetailPrint "Uyarı: kaldırma komutu hata verdi (kod: $0)."
  ${EndIf}
  Delete "$INSTDIR\uninstall.exe"
  Delete "$INSTDIR\discord-dns.exe"
  RMDir "$INSTDIR"
  DeleteRegKey HKLM "${UNINST_KEY}"
  DetailPrint "${APP} kaldırıldı."
SectionEnd
