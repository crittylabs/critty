on run argv
    if item 1 of argv is "close" and application id "com.googlecode.iterm2" is not running then return
    tell application id "com.googlecode.iterm2"
        if item 1 of argv is "open" then
            set reviewWindow to create window with default profile command (item 2 of argv)
            set reviewWindowID to id of reviewWindow
            activate
            return reviewWindowID as text
        else if item 1 of argv is "close" then
            set reviewWindowID to (item 2 of argv) as integer
            ignoring application responses
                close (every window whose id is reviewWindowID)
            end ignoring
        end if
    end tell
end run
