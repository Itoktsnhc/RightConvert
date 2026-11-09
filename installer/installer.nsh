; RightConvert installer hooks. The installer runs per user without
; administrator rights, so these write only HKEY_CURRENT_USER (through
; RightConvert.exe --register, which writes HKCU\Software\Classes).

!macro customInstall
  ExecWait '"$INSTDIR\RightConvert.exe" --register'
!macroend

!macro customUnInstall
  ExecWait '"$INSTDIR\RightConvert.exe" --unregister'
!macroend
