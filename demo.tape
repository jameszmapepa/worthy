# VHS tape for the worthy demo GIF. Render with `vhs demo.tape`
# (https://github.com/charmbracelet/vhs). GITHUB_TOKEN in the environment is
# picked up automatically to lift the API rate limit.
#
# --ascii renders the language tag as plain text (e.g. "Go") instead of a Nerd
# Font devicon, so the GIF does not depend on a Nerd Font being installed in CI.
Output demo.gif

Set FontSize 16
Set Width 1200
Set Height 760
Set Padding 16

Type "worthy --ascii charmbracelet/log"
Enter
Sleep 12s

# Cycle the views with the arrow-key navigation.
Right
Sleep 2s
Right
Sleep 2s
Right
Sleep 2s
Left
Sleep 2s

Type "q"
Sleep 1s
