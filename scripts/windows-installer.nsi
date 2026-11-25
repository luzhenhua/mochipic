Unicode true
ManifestDPIAware true
ManifestSupportedOS Win10
RequestExecutionLevel user
SetCompressor /SOLID lzma
AllowSkipFiles off
!include "MUI2.nsh"
!include "x64.nsh"
!include "WinVer.nsh"
!include "FileFunc.nsh"

!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\MochiPic"

Name "MochiPic"
OutFile "${INSTALLER}"
InstallDir "$LOCALAPPDATA\Programs\MochiPic"
InstallDirRegKey HKCU "${UNINSTALL_KEY}" "UninstallString"
Icon "${APP_ICON}"
UninstallIcon "${APP_ICON}"

!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN "$INSTDIR\MochiPic.exe"
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

LangString ArchitectureError ${LANG_SIMPCHINESE} "此安装包需要 64 位 Windows 10 或更高版本。"
LangString ArchitectureError ${LANG_ENGLISH} "This installer requires Windows 10 or later on a 64-bit system."
LangString FileError ${LANG_SIMPCHINESE} "无法处理安装目录中的文件。请关闭麻薯图图并检查安装路径。"
LangString FileError ${LANG_ENGLISH} "Could not update the files in the installation folder. Close MochiPic and check the installation path."
LangString FolderConflict ${LANG_SIMPCHINESE} "所选目录已有同名文件。请选择一个新的麻薯图图安装目录。"
LangString FolderConflict ${LANG_ENGLISH} "This folder contains conflicting files. Choose another folder for MochiPic."
LangString DesktopName ${LANG_SIMPCHINESE} "麻薯图图"
LangString DesktopName ${LANG_ENGLISH} "MochiPic"

!macro RejectReparse PATH
  System::Call 'kernel32::GetFileAttributesW(w "${PATH}") i.r7'
  ${If} $7 != -1
    IntOp $7 $7 & 0x400
    ${If} $7 != 0
      MessageBox MB_ICONSTOP "$(FileError)" /SD IDOK
      SetErrorLevel 1
      Abort
    ${EndIf}
  ${EndIf}
!macroend

!macro GuardInstallDirectory
  StrCpy $8 "$INSTDIR"
  ${DoWhile} $8 != ""
    !insertmacro RejectReparse "$8"
    ${GetParent} "$8" $9
    ${If} $8 == $9
      ${ExitDo}
    ${EndIf}
    StrCpy $8 $9
  ${Loop}
!macroend

; Check the stored target without resolving or invoking a shortcut.
!macro ReadShortcutTarget PATH RESULT
  StrCpy ${RESULT} ""
  System::Call 'ole32::CoCreateInstance(g"{00021401-0000-0000-C000-000000000046}", p 0, i 1, g"{000214F9-0000-0000-C000-000000000046}", *p.r2) i.r1'
  ${If} $1 == 0
    System::Call '$2->0(g"{0000010B-0000-0000-C000-000000000046}", *p.r3) i.r1'
    ${If} $1 == 0
      System::Call '$3->5(w "${PATH}", i 0) i.r1'
      ${If} $1 == 0
        ; The item ID list retains Unicode paths even with legacy link data.
        System::Call '$2->4(*p.R1) i.r1'
        ${If} $1 == 0
        ${AndIf} $R1 != 0
          System::Call 'shell32::SHGetPathFromIDListW(p R1, w .r4) i.r1'
          ${If} $1 != 0
            StrCpy ${RESULT} $4
          ${EndIf}
          System::Call 'ole32::CoTaskMemFree(p R1)'
        ${EndIf}
        ${If} ${RESULT} == ""
          System::Call '$2->3(w .r4, i ${NSIS_MAX_STRLEN}, p 0, i 4) i.r1'
          ${If} $1 == 0
            StrCpy ${RESULT} $4
          ${EndIf}
        ${EndIf}
      ${EndIf}
      System::Call '$3->2()'
    ${EndIf}
    System::Call '$2->2()'
  ${EndIf}
!macroend

!macro RemoveAppShortcut PATH
  ${If} ${FileExists} "${PATH}"
    !insertmacro ReadShortcutTarget "${PATH}" $5
    ${If} $5 == "$INSTDIR\MochiPic.exe"
      Delete "${PATH}"
    ${EndIf}
  ${EndIf}
!macroend

!macro InstallAppShortcut PATH
  ${If} ${FileExists} "${PATH}"
    !insertmacro ReadShortcutTarget "${PATH}" $5
    ${If} $5 == "$INSTDIR\MochiPic.exe"
    ${OrIf} $5 == "$6\MochiPic.exe"
      CreateShortCut "${PATH}" "$INSTDIR\MochiPic.exe"
    ${EndIf}
  ${Else}
    CreateShortCut "${PATH}" "$INSTDIR\MochiPic.exe"
  ${EndIf}
!macroend

Function .onInit
  ${IfNot} ${RunningX64}
  ${OrIfNot} ${AtLeastWin10}
    MessageBox MB_ICONSTOP "$(ArchitectureError)"
    Abort
  ${EndIf}
FunctionEnd

Section "Install"
  !insertmacro GuardInstallDirectory
  !include "${PATH_GUARDS}"
  ${If} ${FileExists} "$INSTDIR\MochiPic.exe"
  ${OrIf} ${FileExists} "$INSTDIR\Uninstall.exe"
  ${OrIf} ${FileExists} "$INSTDIR\current-runtime"
    ReadRegStr $0 HKCU "${UNINSTALL_KEY}" "UninstallString"
    ${If} $0 != '$\"$INSTDIR\Uninstall.exe$\"'
      MessageBox MB_ICONSTOP "$(FolderConflict)" /SD IDOK
      SetErrorLevel 1
      Abort
    ${EndIf}
  ${EndIf}
  ClearErrors
  SetOutPath "$INSTDIR\${RUNTIME_DIRECTORY}"
  SetOverwrite off
  File /r "${PAYLOAD}/${RUNTIME_DIRECTORY}/*"
  SetOverwrite on
  IfErrors install_failed
  SetOutPath "$INSTDIR"
  SetOverwrite off
  File "${PAYLOAD}/.runtime-lock"
  SetOverwrite on
  IfErrors install_failed
  File /oname=.activate-${RUNTIME_DIRECTORY} "${PAYLOAD}/current-runtime"
  IfErrors install_failed
  System::Call 'kernel32::MoveFileExW(w "$INSTDIR\.activate-${RUNTIME_DIRECTORY}", w "$INSTDIR\current-runtime", i 9) i.r7'
  ${If} $7 == 0
    Goto install_failed
  ${EndIf}
  File "${PAYLOAD}/MochiPic.exe"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  IfErrors install_failed
  ReadRegStr $6 HKCU "${UNINSTALL_KEY}" "InstallLocation"
  !insertmacro InstallAppShortcut "$SMPROGRAMS\MochiPic.lnk"
  !insertmacro InstallAppShortcut "$DESKTOP\$(DesktopName).lnk"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayName" "MochiPic"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayVersion" "${APP_VERSION}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "UninstallString" '$\"$INSTDIR\Uninstall.exe$\"'
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayIcon" "$INSTDIR\MochiPic.exe"
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoRepair" 1
  Goto install_done
install_failed:
  MessageBox MB_ICONSTOP "$(FileError)" /SD IDOK
  SetErrorLevel 1
  Abort
install_done:
SectionEnd

Section "Uninstall"
  !insertmacro GuardInstallDirectory
  !insertmacro RejectReparse "$INSTDIR\MochiPic.exe"
  !insertmacro RejectReparse "$INSTDIR\Uninstall.exe"
  ClearErrors
  ExecWait '"$INSTDIR\MochiPic.exe" --update-cleanup' $0
  IfErrors uninstall_failed
  ${If} $0 != 0
    Goto uninstall_failed
  ${EndIf}
  !insertmacro RemoveAppShortcut "$SMPROGRAMS\MochiPic.lnk"
  !insertmacro RemoveAppShortcut "$DESKTOP\MochiPic.lnk"
  !insertmacro RemoveAppShortcut "$DESKTOP\麻薯图图.lnk"
  ClearErrors
  Delete "$INSTDIR\MochiPic.exe"
  IfErrors uninstall_failed
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
  ReadRegStr $0 HKCU "${UNINSTALL_KEY}" "UninstallString"
  ${If} $0 == '$\"$INSTDIR\Uninstall.exe$\"'
    DeleteRegKey HKCU "${UNINSTALL_KEY}"
  ${EndIf}
  Goto uninstall_done
uninstall_failed:
  MessageBox MB_ICONSTOP "$(FileError)" /SD IDOK
  SetErrorLevel 1
  Abort
uninstall_done:
SectionEnd
