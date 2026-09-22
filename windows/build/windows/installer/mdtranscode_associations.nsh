# MDTranscode Windows shell registration.
#
# Wails v2's stock APP_ASSOCIATE macro writes the extension's default ProgID.
# MDTranscode intentionally does not do that. Windows/user choice remains in
# control of the default Markdown application.

!define MDT_PROGID "MDTranscode.Markdown"
!define MDT_REGISTERED_APP "MDTranscode"
!define MDT_CAPABILITIES_PATH "Software\MDTranscode\Capabilities"

!macro mdtranscode.associateFiles
    SetRegView 64

    # Install the file-type icon beside the application executable.
    SetOutPath $INSTDIR
    File "..\appicon.ico"

    # Product-specific ProgID used by Open With and Default Apps.
    WriteRegStr SHELL_CONTEXT "Software\Classes\${MDT_PROGID}" "" "Markdown Document"
    WriteRegStr SHELL_CONTEXT "Software\Classes\${MDT_PROGID}\DefaultIcon" "" "$INSTDIR\appicon.ico"
    WriteRegStr SHELL_CONTEXT "Software\Classes\${MDT_PROGID}\shell" "" "open"
    WriteRegStr SHELL_CONTEXT "Software\Classes\${MDT_PROGID}\shell\open" "" "Open with ${INFO_PRODUCTNAME}"
    WriteRegStr SHELL_CONTEXT "Software\Classes\${MDT_PROGID}\shell\open\command" "" "$\"$INSTDIR\${PRODUCT_EXECUTABLE}$\" $\"%1$\""

    # Offer MDTranscode for these extensions without replacing their defaults.
    WriteRegStr SHELL_CONTEXT "Software\Classes\.md\OpenWithProgids" "${MDT_PROGID}" ""
    WriteRegStr SHELL_CONTEXT "Software\Classes\.markdown\OpenWithProgids" "${MDT_PROGID}" ""

    # Register the executable as an Open With application and declare its types.
    WriteRegStr SHELL_CONTEXT "Software\Classes\Applications\${PRODUCT_EXECUTABLE}" "FriendlyAppName" "${INFO_PRODUCTNAME}"
    WriteRegStr SHELL_CONTEXT "Software\Classes\Applications\${PRODUCT_EXECUTABLE}\DefaultIcon" "" "$INSTDIR\${PRODUCT_EXECUTABLE},0"
    WriteRegStr SHELL_CONTEXT "Software\Classes\Applications\${PRODUCT_EXECUTABLE}\SupportedTypes" ".md" ""
    WriteRegStr SHELL_CONTEXT "Software\Classes\Applications\${PRODUCT_EXECUTABLE}\SupportedTypes" ".markdown" ""
    WriteRegStr SHELL_CONTEXT "Software\Classes\Applications\${PRODUCT_EXECUTABLE}\shell\open\command" "" "$\"$INSTDIR\${PRODUCT_EXECUTABLE}$\" $\"%1$\""

    # Register with the Windows Default Apps platform. This makes MDTranscode a
    # candidate; it does not set it as the user's default handler.
    WriteRegStr SHELL_CONTEXT "${MDT_CAPABILITIES_PATH}" "ApplicationName" "${INFO_PRODUCTNAME}"
    WriteRegStr SHELL_CONTEXT "${MDT_CAPABILITIES_PATH}" "ApplicationDescription" "Turn Markdown into professional documents."
    WriteRegStr SHELL_CONTEXT "${MDT_CAPABILITIES_PATH}\FileAssociations" ".md" "${MDT_PROGID}"
    WriteRegStr SHELL_CONTEXT "${MDT_CAPABILITIES_PATH}\FileAssociations" ".markdown" "${MDT_PROGID}"
    WriteRegStr SHELL_CONTEXT "Software\RegisteredApplications" "${MDT_REGISTERED_APP}" "${MDT_CAPABILITIES_PATH}"

    # Add a stable secondary conversion verb for Markdown regardless of which
    # application owns the default file association. On Windows 11 this static
    # Win32 verb appears in the classic "Show more options" menu.
    WriteRegStr SHELL_CONTEXT "Software\Classes\SystemFileAssociations\.md\shell\MDTranscode.ConvertToWord" "" "Convert to Word with MDTranscode"
    WriteRegStr SHELL_CONTEXT "Software\Classes\SystemFileAssociations\.md\shell\MDTranscode.ConvertToWord" "Icon" "$INSTDIR\${PRODUCT_EXECUTABLE},0"
    WriteRegStr SHELL_CONTEXT "Software\Classes\SystemFileAssociations\.md\shell\MDTranscode.ConvertToWord" "MultiSelectModel" "Single"
    WriteRegStr SHELL_CONTEXT "Software\Classes\SystemFileAssociations\.md\shell\MDTranscode.ConvertToWord\command" "" "$\"$INSTDIR\${PRODUCT_EXECUTABLE}$\" --convert-to-docx $\"%1$\""

    WriteRegStr SHELL_CONTEXT "Software\Classes\SystemFileAssociations\.markdown\shell\MDTranscode.ConvertToWord" "" "Convert to Word with MDTranscode"
    WriteRegStr SHELL_CONTEXT "Software\Classes\SystemFileAssociations\.markdown\shell\MDTranscode.ConvertToWord" "Icon" "$INSTDIR\${PRODUCT_EXECUTABLE},0"
    WriteRegStr SHELL_CONTEXT "Software\Classes\SystemFileAssociations\.markdown\shell\MDTranscode.ConvertToWord" "MultiSelectModel" "Single"
    WriteRegStr SHELL_CONTEXT "Software\Classes\SystemFileAssociations\.markdown\shell\MDTranscode.ConvertToWord\command" "" "$\"$INSTDIR\${PRODUCT_EXECUTABLE}$\" --convert-to-docx $\"%1$\""

    # Notify Explorer immediately that file-association metadata changed.
    System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'
!macroend

!macro mdtranscode.unassociateFiles
    SetRegView 64

    # Remove only MDTranscode's candidate registrations. Do not touch the
    # extension default value or another application's ProgID.
    DeleteRegValue SHELL_CONTEXT "Software\Classes\.md\OpenWithProgids" "${MDT_PROGID}"
    DeleteRegValue SHELL_CONTEXT "Software\Classes\.markdown\OpenWithProgids" "${MDT_PROGID}"
    DeleteRegKey SHELL_CONTEXT "Software\Classes\SystemFileAssociations\.md\shell\MDTranscode.ConvertToWord"
    DeleteRegKey SHELL_CONTEXT "Software\Classes\SystemFileAssociations\.markdown\shell\MDTranscode.ConvertToWord"
    DeleteRegKey SHELL_CONTEXT "Software\Classes\Applications\${PRODUCT_EXECUTABLE}"
    DeleteRegKey SHELL_CONTEXT "Software\Classes\${MDT_PROGID}"
    DeleteRegValue SHELL_CONTEXT "Software\RegisteredApplications" "${MDT_REGISTERED_APP}"
    DeleteRegKey SHELL_CONTEXT "${MDT_CAPABILITIES_PATH}"
    DeleteRegKey /ifempty SHELL_CONTEXT "Software\MDTranscode"

    System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'
!macroend
