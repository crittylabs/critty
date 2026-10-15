on run argv
    if item 1 of argv is "close" and application id "com.mitchellh.ghostty" is not running then return
    tell application id "com.mitchellh.ghostty"
        if item 1 of argv is "open" then
            set config to new surface configuration
            set command of config to item 2 of argv
            set initial working directory of config to item 3 of argv
            set wait after command of config to false
            set reviewWindow to new window with configuration config
            set reviewWindowID to id of reviewWindow
            activate window (first window whose id is reviewWindowID)
            return reviewWindowID as text
        else if item 1 of argv is "close" then
            set reviewWindowID to item 2 of argv
            if exists (first window whose id is reviewWindowID) then
                ignoring application responses
                    close window (first window whose id is reviewWindowID)
                end ignoring
            end if
        end if
    end tell
end run
