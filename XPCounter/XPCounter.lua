-- XP Counter
-- A small, clean XP tracker for WoW: Forever.
-- Shows current XP, XP left to level, XP per hour, estimated time to level, a session timer
-- and the XP waiting in completed quests you haven't turned in yet.
--
-- Drag to move, right-click for settings, Shift + right-click to reset the session.
-- Hovering the card shows a pause button. The settings are also under
-- Esc > Options > AddOns (see Settings.lua). Optionally hides the game's own XP bar.

local ADDON_NAME, ns = ...

---------------------------------------------------------------------------
-- Settings and look
---------------------------------------------------------------------------

local DEFAULTS = {
    point = "CENTER", relPoint = "CENTER", x = 0, y = 200,
    scale = 1,
    alpha = 0.82,            -- background opacity
    accent = "violet",
    locked = false,
    hidden = false,
    hideInCombat = false,
    showXP = true, showLeft = true, showRate = true, showEta = true, showTimer = true,
    showQuests = true,       -- XP waiting in completed quests
    showBar = true, showRested = true,
    hideGameBar = false,     -- hide the game's own XP bar
}

local WIDTH      = 200
local PAD        = 10
local ROW_HEIGHT = 16
local BAR_Y      = PAD + 19
local BAR_HEIGHT = 4
local MIN_SAMPLE = 60   -- seconds of play before XP/hour is shown
local TICK       = 0.25 -- seconds between refreshes, so the session timer ticks over smoothly
local FONT       = STANDARD_TEXT_FONT or "Fonts\\FRIZQT__.TTF"
local WHITE      = "Interface\\Buttons\\WHITE8x8"

local C = {
    bg    = { 0.04, 0.04, 0.06 },
    track = { 1, 1, 1, 0.08 },
    label = { 0.60, 0.60, 0.65 },
    value = { 0.95, 0.95, 0.95 },
    quest = { 1.00, 0.82, 0.30 },   -- quest XP section on the bar
    ready = { 0.45, 0.90, 0.50 },   -- quest XP that would level you up
    hint  = { 0.50, 0.50, 0.55 },
}

local ACCENTS = {
    { key = "violet", name = "Violet", r = 0.67, g = 0.47, b = 1.00 },
    { key = "blue",   name = "Blue",   r = 0.35, g = 0.65, b = 1.00 },
    { key = "teal",   name = "Teal",   r = 0.25, g = 0.82, b = 0.76 },
    { key = "gold",   name = "Gold",   r = 1.00, g = 0.78, b = 0.30 },
    { key = "rose",   name = "Rose",   r = 1.00, g = 0.47, b = 0.58 },
}

ns.C, ns.WHITE, ns.ACCENTS = C, WHITE, ACCENTS

---------------------------------------------------------------------------
-- State
---------------------------------------------------------------------------

local db, charDB
-- active = seconds played this session, not counting AFK or paused time
local session = { gained = 0, active = 0, levels = 0, paused = false }
local lastXP, lastMax, lastTick
local ticker
local inCombat = false
local quests = { xp = 0, count = 0, known = false }   -- completed quests not yet turned in

---------------------------------------------------------------------------
-- Helpers
---------------------------------------------------------------------------

local function Print(msg)
    print("|cffab78ffXP Counter|r " .. msg)
end

-- The Midnight-era client can hand addons "secret" values that may be shown but not used in math.
-- XP isn't expected to be one; if it ever is, that update is skipped instead of throwing errors.
local issecretvalue = issecretvalue
local function Readable(v)
    return v ~= nil and not (issecretvalue and issecretvalue(v))
end

local function ReadXP()
    local cur, max = UnitXP("player"), UnitXPMax("player")
    if Readable(cur) and Readable(max) then
        return cur, max
    end
end

local function ReadRested()
    local rested = GetXPExhaustion()
    return Readable(rested) and rested or 0
end

local function AtMaxLevel()
    if IsXPUserDisabled and IsXPUserDisabled() then return true end
    if IsPlayerAtEffectiveMaxLevel then return IsPlayerAtEffectiveMaxLevel() end
    local maxLevel = (GetMaxLevelForPlayerExpansion and GetMaxLevelForPlayerExpansion())
        or (GetMaxPlayerLevel and GetMaxPlayerLevel())
    return maxLevel ~= nil and UnitLevel("player") >= maxLevel
end

local function FormatNumber(n)
    local s = tostring(math.floor(n + 0.5))
    s = s:reverse():gsub("(%d%d%d)", "%1,"):reverse()
    return (s:gsub("^,", ""))
end

local function FormatDuration(seconds)
    seconds = math.floor(seconds + 0.5)
    local d = math.floor(seconds / 86400)
    local h = math.floor(seconds % 86400 / 3600)
    local m = math.floor(seconds % 3600 / 60)
    if d > 0 then return string.format("%dd %dh", d, h) end
    if h > 0 then return string.format("%dh %02dm", h, m) end
    if m > 0 then return string.format("%dm", m) end
    return string.format("%ds", seconds)
end

-- Stopwatch style: 4:07, 23:45, 1:02:05
local function FormatClock(seconds)
    seconds = math.floor(seconds)
    local h = math.floor(seconds / 3600)
    local m = math.floor(seconds % 3600 / 60)
    local s = seconds % 60
    if h > 0 then return string.format("%d:%02d:%02d", h, m, s) end
    return string.format("%d:%02d", m, s)
end

local function XPPerSecond()
    if session.active >= MIN_SAMPLE and session.gained > 0 then
        return session.gained / session.active
    end
end

local function Accent()
    local key = db and db.accent
    for _, accent in ipairs(ACCENTS) do
        if accent.key == key then return accent end
    end
    return ACCENTS[1]
end

local function NewText(parent, size, color, justify)
    local fs = parent:CreateFontString(nil, "OVERLAY")
    fs:SetFont(FONT, size, "")
    fs:SetShadowOffset(1, -1)
    fs:SetShadowColor(0, 0, 0, 0.75)
    fs:SetTextColor(color[1], color[2], color[3])
    fs:SetJustifyH(justify or "LEFT")
    return fs
end

ns.Print, ns.Accent, ns.NewText = Print, Accent, NewText

---------------------------------------------------------------------------
-- The card
---------------------------------------------------------------------------

local frame = CreateFrame("Frame", "XPCounterFrame", UIParent, "BackdropTemplate")
frame:SetWidth(WIDTH)
frame:SetFrameStrata("LOW")
frame:SetClampedToScreen(true)
frame:SetMovable(true)
frame:EnableMouse(true)
frame:RegisterForDrag("LeftButton")
frame:SetBackdrop({ bgFile = WHITE, edgeFile = WHITE, edgeSize = 1 })
frame:Hide()
ns.card = frame

-- Header: level (plus "Paused" while paused) on the left, percentage on the right
local levelText = NewText(frame, 13, C.value)
levelText:SetPoint("TOPLEFT", PAD, -PAD)
local pausedText = NewText(frame, 11, C.hint)
pausedText:SetPoint("BOTTOMLEFT", levelText, "BOTTOMRIGHT", 6, 0)
pausedText:SetText("Paused")
pausedText:Hide()
local percentText = NewText(frame, 12, C.value, "RIGHT")
percentText:SetPoint("TOPRIGHT", -PAD, -PAD - 1)

-- Pause button: two small bars beside the percentage. Shown while the mouse is over the card,
-- and always while paused (then in the accent color).
local pauseButton = CreateFrame("Button", nil, frame)
pauseButton:SetSize(14, 14)
pauseButton:SetPoint("RIGHT", percentText, "LEFT", -5, 0)
pauseButton:Hide()
local pauseBars = {}
for i, x in ipairs({ 3, 8 }) do
    local bar = pauseButton:CreateTexture(nil, "ARTWORK")
    bar:SetSize(3, 10)
    bar:SetPoint("TOPLEFT", x, -2)
    pauseBars[i] = bar
end

local function RefreshPauseButton()
    pauseButton:SetShown(session.paused or frame:IsMouseOver())
    local a = Accent()
    for _, bar in ipairs(pauseBars) do
        if session.paused then
            bar:SetColorTexture(a.r, a.g, a.b, 1)
        else
            bar:SetColorTexture(1, 1, 1, pauseButton:IsMouseOver() and 0.9 or 0.45)
        end
    end
end

-- Thin progress bar, with rested XP shown as a fainter section ahead of it
local track = frame:CreateTexture(nil, "ARTWORK")
track:SetColorTexture(unpack(C.track))
track:SetPoint("TOPLEFT", PAD, -BAR_Y)
track:SetPoint("TOPRIGHT", -PAD, -BAR_Y)
track:SetHeight(BAR_HEIGHT)

local function NewBar(level)
    local bar = CreateFrame("StatusBar", nil, frame)
    bar:SetAllPoints(track)
    bar:SetStatusBarTexture(WHITE)
    bar:SetFrameLevel(frame:GetFrameLevel() + level)
    return bar
end
local restedBar = NewBar(1)
local questBar = NewBar(2)   -- where turning in your completed quests would put you
local xpBar = NewBar(3)

-- Rows: label on the left, value on the right
local ROWS = {
    { key = "xp",   label = "XP",            setting = "showXP" },
    { key = "left", label = "To level",      setting = "showLeft" },
    { key = "rate", label = "XP / hour",     setting = "showRate" },
    { key = "eta",  label = "Time to level", setting = "showEta" },
    { key = "quests", label = "Quests ready", setting = "showQuests" },
    { key = "timer", label = "Session",      setting = "showTimer" },
}
local values = {}
for _, row in ipairs(ROWS) do
    row.labelText = NewText(frame, 11, C.label)
    row.labelText:SetText(row.label)
    row.valueText = NewText(frame, 11, C.value, "RIGHT")
    values[row.key] = row.valueText
end

-- Stacks the bar and the enabled rows under the header, and sizes the card to fit.
local function Layout()
    local bottom = PAD + 13             -- bottom of the header
    local y = bottom + 8                -- where the first row goes
    if db.showBar then
        bottom = BAR_Y + BAR_HEIGHT
        y = bottom + 9
    end
    track:SetShown(db.showBar)
    xpBar:SetShown(db.showBar)
    restedBar:SetShown(db.showBar and db.showRested)
    questBar:SetShown(db.showBar and db.showQuests)

    for _, row in ipairs(ROWS) do
        local shown = db[row.setting]
        row.labelText:SetShown(shown)
        row.valueText:SetShown(shown)
        if shown then
            row.labelText:ClearAllPoints()
            row.labelText:SetPoint("TOPLEFT", PAD, -y)
            row.valueText:ClearAllPoints()
            row.valueText:SetPoint("TOPRIGHT", -PAD, -y)
            bottom = y + 12
            y = y + ROW_HEIGHT
        end
    end
    frame:SetHeight(bottom + PAD)
end

local function ApplyLook()
    local a = Accent()
    frame:SetBackdropColor(C.bg[1], C.bg[2], C.bg[3], db.alpha)
    frame:SetBackdropBorderColor(1, 1, 1, db.alpha * 0.1)
    xpBar:SetStatusBarColor(a.r, a.g, a.b, 1)
    restedBar:SetStatusBarColor(a.r, a.g, a.b, 0.35)
    questBar:SetStatusBarColor(C.quest[1], C.quest[2], C.quest[3], 0.6)
    percentText:SetTextColor(a.r, a.g, a.b)
    RefreshPauseButton()
end

---------------------------------------------------------------------------
-- Completed quests waiting to be turned in
---------------------------------------------------------------------------

local function QuestReady(questID)
    local QL = C_QuestLog
    if QL.ReadyForTurnIn then
        local ready = QL.ReadyForTurnIn(questID)
        if Readable(ready) and ready ~= nil then return ready end
    end
    if QL.IsComplete then
        local done = QL.IsComplete(questID)
        if Readable(done) then return done end
    end
    return false
end

-- Adds up the reward XP of every quest in your log that's ready to hand in.
-- (The game works out each reward for your current level, like the quest log shows it.)
local function ScanQuests()
    local QL = C_QuestLog
    if not (QL and QL.GetNumQuestLogEntries and QL.GetInfo and GetQuestLogRewardXP) then
        quests.known = false
        return
    end
    local ok = pcall(function()
        local xp, count = 0, 0
        local num = QL.GetNumQuestLogEntries()
        for i = 1, (Readable(num) and num) or 0 do
            local info = QL.GetInfo(i)
            local id = type(info) == "table" and not info.isHeader and not info.isHidden and info.questID
            if id and Readable(id) and QuestReady(id) then
                local reward = GetQuestLogRewardXP(id)
                if Readable(reward) and type(reward) == "number" and reward > 0 then
                    xp, count = xp + reward, count + 1
                end
            end
        end
        quests.xp, quests.count, quests.known = xp, count, true
    end)
    if not ok then quests.known = false end   -- keep the card working if the quest API changes
    if not quests.known then quests.xp, quests.count = 0, 0 end
end

---------------------------------------------------------------------------
-- Tracking and display
---------------------------------------------------------------------------

local function Update()
    if not db then return end
    local cur, max = ReadXP()
    if not cur then return end

    if db.hidden or (db.hideInCombat and inCombat) or max <= 0 or AtMaxLevel() then
        frame:Hide()
        return
    end

    local left = max - cur
    local perSecond = XPPerSecond()
    local rateColor = session.paused and C.label or C.value   -- dimmed while paused

    levelText:SetText("Level " .. UnitLevel("player"))
    pausedText:SetShown(session.paused)
    percentText:SetText(string.format("%.1f%%", cur / max * 100))

    xpBar:SetMinMaxValues(0, max)
    xpBar:SetValue(cur)
    restedBar:SetMinMaxValues(0, max)
    restedBar:SetValue(math.min(cur + ReadRested(), max))
    questBar:SetMinMaxValues(0, max)
    questBar:SetValue(math.min(cur + quests.xp, max))

    values.xp:SetText(FormatNumber(cur) .. " / " .. FormatNumber(max))
    values.left:SetText(FormatNumber(left))
    values.rate:SetText(perSecond and FormatNumber(perSecond * 3600) or "-")
    values.eta:SetText(perSecond and FormatDuration(left / perSecond) or "-")
    values.timer:SetText(FormatClock(session.active))
    local questColor = (quests.xp > 0 and quests.xp >= left) and C.ready or C.value
    values.quests:SetText(quests.known and FormatNumber(quests.xp) or "-")
    values.quests:SetTextColor(questColor[1], questColor[2], questColor[3])
    for _, key in ipairs({ "rate", "eta", "timer" }) do
        values[key]:SetTextColor(rateColor[1], rateColor[2], rateColor[3])
    end

    frame:Show()
    RefreshPauseButton()
end

-- Adds XP gained since the last check to the session (skipped while paused).
local function TrackXP()
    local cur, max = ReadXP()
    if not cur then return end
    if lastXP and not session.paused then
        if cur < lastXP or max ~= lastMax then
            -- Levelled up: finish off the old level, then count progress into the new one.
            session.gained = session.gained + (lastMax - lastXP) + cur
        else
            session.gained = session.gained + (cur - lastXP)
        end
    end
    lastXP, lastMax = cur, max
end

local function StartSession()
    session.gained, session.active, session.levels, session.paused = 0, 0, 0, false
    lastXP, lastMax = ReadXP()
    lastTick = GetTime()
end

local function ResetSession()
    StartSession()
    Update()
    ns.RefreshSettings()
    Print("session reset.")
end

---------------------------------------------------------------------------
-- Optional: hiding the game's own XP bar
--
-- In Forever the game's XP bar sits in one of two bar containers inside
-- StatusTrackingBarManager (the other one can show e.g. a watched reputation),
-- and those containers are part of the bottom action bar layout. Calling Hide()
-- on them would make Blizzard's code move the secure action bars from addon code,
-- which can cause "action blocked" errors. So this only uses transparency:
--   * the manager is made fully transparent,
--   * a container showing anything other than XP ignores that and stays visible,
--   * the XP bars ignore the mouse, so hovering the empty spot shows no tooltip.
-- Blizzard's own logic keeps running untouched. Nothing is hidden while Edit Mode
-- is open, so the bar can still be moved there.
---------------------------------------------------------------------------

local gameBar = { hooked = false, active = false, warned = false, alphaBefore = 1, mouseOff = {} }

local function GameBarManager()
    local manager = StatusTrackingBarManager
    if type(manager) == "table" and type(manager.barContainers) == "table" then
        return manager
    end
end

local function EditModeOpen()
    local editMode = EditModeManagerFrame
    return editMode and editMode.IsEditModeActive and editMode:IsEditModeActive() and true or false
end

local function ShowsXP(container)
    local bars = container.bars
    local shown = type(bars) == "table" and bars[container.shownBarIndex]
    return type(shown) == "table" and shown.isExpBar == true
end

local function MouseOffFrame(f)
    if f and f.IsMouseEnabled and f:IsMouseEnabled() then
        f:EnableMouse(false)
        gameBar.mouseOff[f] = true   -- remembered, so exactly these get it back
    end
end

local function MouseOff(bar)
    MouseOffFrame(bar)
    MouseOffFrame(bar.ExhaustionTick)   -- the rested XP marker has its own tooltip
end

local function ApplyGameBar()
    if not db then return end
    local manager = GameBarManager()
    if not manager then
        if db.hideGameBar and not gameBar.warned then
            gameBar.warned = true
            Print("couldn't find the game's XP bar to hide in this version of the game.")
        end
        return
    end

    if not db.hideGameBar or EditModeOpen() then
        if gameBar.active then   -- put back exactly what this addon changed
            manager:SetAlpha(gameBar.alphaBefore)
            for _, container in ipairs(manager.barContainers) do
                container:SetIgnoreParentAlpha(false)
            end
            for f in pairs(gameBar.mouseOff) do f:EnableMouse(true) end
            gameBar.mouseOff = {}
            gameBar.active = false
        end
        return
    end

    if not gameBar.active then
        gameBar.alphaBefore = manager:GetAlpha()
        gameBar.active = true
    end
    if manager:GetAlpha() ~= 0 then manager:SetAlpha(0) end
    for _, container in ipairs(manager.barContainers) do
        local keepVisible = not ShowsXP(container)
        if container:IsIgnoringParentAlpha() ~= keepVisible then
            container:SetIgnoreParentAlpha(keepVisible)
        end
        if type(container.bars) == "table" then
            for _, bar in pairs(container.bars) do
                if type(bar) == "table" and bar.isExpBar then MouseOff(bar) end
            end
        end
    end
end

-- Re-checks right after Blizzard swaps which bar a container shows, and when Edit Mode opens or closes.
local function SetupGameBar()
    local manager = GameBarManager()
    if gameBar.hooked or not manager then return end
    gameBar.hooked = true
    for _, container in ipairs(manager.barContainers) do
        if type(container.ApplyPendingBarToShow) == "function" then
            hooksecurefunc(container, "ApplyPendingBarToShow", ApplyGameBar)
        end
    end
    if EventRegistry and EventRegistry.RegisterCallback then
        EventRegistry:RegisterCallback("EditMode.Enter", ApplyGameBar, frame)
        EventRegistry:RegisterCallback("EditMode.Exit", ApplyGameBar, frame)
    end
end

-- Runs a few times a second: counts play time (not while AFK or paused) and refreshes the numbers.
local function Tick()
    local now = GetTime()
    if lastTick and not session.paused and UnitIsAFK("player") ~= true then
        session.active = session.active + (now - lastTick)
    end
    lastTick = now
    TrackXP()
    Update()
    if db and (db.hideGameBar or gameBar.active) then
        ApplyGameBar()   -- cheap safety net in case anything else changed the bar
    end
end

-- Pausing freezes the session: time and XP from while it's paused don't count.
local function TogglePause()
    Tick()   -- settle the session up to this moment first
    session.paused = not session.paused
    Update()
    ns.RefreshSettings()
end

local function IsPaused()
    return session.paused
end

---------------------------------------------------------------------------
-- Position and applying settings
---------------------------------------------------------------------------

local function ApplyPosition()
    frame:ClearAllPoints()
    frame:SetPoint(db.point, UIParent, db.relPoint, db.x, db.y)
end

local function SavePosition()
    local point, _, relPoint, x, y = frame:GetPoint(1)
    db.point, db.relPoint, db.x, db.y = point, relPoint, x, y
end

local function CenterCard()
    db.point, db.relPoint, db.x, db.y = "CENTER", "CENTER", 0, 0
    ApplyPosition()
end

-- Rescales the card around its anchor point, so it doesn't drift across the screen.
local function ApplyScale()
    local old = frame:GetScale()
    if old == db.scale then return end
    db.x, db.y = db.x * old / db.scale, db.y * old / db.scale
    frame:SetScale(db.scale)
    ApplyPosition()
end

local function ApplySettings()
    ApplyScale()
    ApplyLook()
    Layout()
    Update()
    ApplyGameBar()
end

ns.ApplySettings, ns.ResetSession, ns.CenterCard = ApplySettings, ResetSession, CenterCard
ns.TogglePause, ns.IsPaused = TogglePause, IsPaused

---------------------------------------------------------------------------
-- Mouse: drag, right-click menu, tooltips, pause button
---------------------------------------------------------------------------

frame:SetScript("OnDragStart", function(self)
    if not db.locked then self:StartMoving() end
end)

frame:SetScript("OnDragStop", function(self)
    self:StopMovingOrSizing()
    self:SetUserPlaced(false)   -- the position is saved by the addon, not the game's layout cache
    SavePosition()
end)

frame:SetScript("OnMouseUp", function(_, button)
    if button ~= "RightButton" then return end
    GameTooltip:Hide()
    if IsShiftKeyDown() then
        ResetSession()
    else
        ns.ToggleSettings()
    end
end)

local function TooltipRow(label, value)
    GameTooltip:AddDoubleLine(label, value,
        C.label[1], C.label[2], C.label[3], C.value[1], C.value[2], C.value[3])
end

local function TooltipHint(text)
    GameTooltip:AddLine(text, C.hint[1], C.hint[2], C.hint[3])
end

frame:SetScript("OnEnter", function(self)
    RefreshPauseButton()
    local a = Accent()
    GameTooltip:SetOwner(self, "ANCHOR_BOTTOM", 0, -4)
    GameTooltip:AddLine("XP Counter", a.r, a.g, a.b)
    local played = FormatClock(session.active)
    TooltipRow("Session time", session.paused and (played .. " (paused)") or played)
    TooltipRow("XP this session", FormatNumber(session.gained))
    if session.levels > 0 then
        TooltipRow("Levels this session", tostring(session.levels))
    end
    local _, max = ReadXP()
    local rested = ReadRested()
    if rested > 0 and max and max > 0 then
        TooltipRow("Rested", string.format("%s (%d%%)", FormatNumber(rested), math.floor(rested / max * 100 + 0.5)))
    else
        TooltipRow("Rested", "None")
    end
    if quests.known and quests.count > 0 then
        TooltipRow("Quests ready", string.format("%d (%s XP)", quests.count, FormatNumber(quests.xp)))
        local cur, max = ReadXP()
        if cur and quests.xp >= max - cur then
            GameTooltip:AddLine("Turning them in levels you up", C.ready[1], C.ready[2], C.ready[3])
        end
    elseif quests.known then
        TooltipRow("Quests ready", "None")
    end
    GameTooltip:AddLine(" ")
    TooltipHint(db.locked and "Position locked" or "Drag to move")
    TooltipHint("Right-click for settings")
    TooltipHint("Shift + right-click to reset the session")
    GameTooltip:Show()
end)

frame:SetScript("OnLeave", function()
    GameTooltip:Hide()
    RefreshPauseButton()   -- stays visible if the mouse only moved onto the pause button
end)

local function ShowPauseTooltip(self)
    GameTooltip:SetOwner(self, "ANCHOR_TOP")
    GameTooltip:AddLine(session.paused and "Resume session" or "Pause session", 1, 1, 1)
    GameTooltip:Show()
end

pauseButton:SetScript("OnClick", function(self)
    TogglePause()
    ShowPauseTooltip(self)
end)

pauseButton:SetScript("OnEnter", function(self)
    RefreshPauseButton()
    ShowPauseTooltip(self)
end)

pauseButton:SetScript("OnLeave", function()
    GameTooltip:Hide()
    RefreshPauseButton()
end)

---------------------------------------------------------------------------
-- Slash command: /xpc opens the settings; a few shortcuts still work
---------------------------------------------------------------------------

SLASH_XPCOUNTER1 = "/xpc"
SLASH_XPCOUNTER2 = "/xpcounter"
SlashCmdList.XPCOUNTER = function(msg)
    local cmd, arg = (msg or ""):lower():match("^%s*(%S*)%s*(.-)%s*$")
    if cmd == "" or cmd == "settings" or cmd == "options" or cmd == "config" then
        ns.ToggleSettings()
        return
    elseif cmd == "reset" then
        ResetSession()
        return
    elseif cmd == "pause" or cmd == "resume" then
        if session.paused ~= (cmd == "pause") then TogglePause() end
        Print(session.paused and "session paused." or "session running.")
        return
    elseif cmd == "lock" or cmd == "unlock" then
        db.locked = (cmd == "lock")
    elseif cmd == "scale" and tonumber(arg) then
        db.scale = math.max(0.5, math.min(2, tonumber(arg)))
    elseif cmd == "hide" or cmd == "show" then
        db.hidden = (cmd == "hide")
        if cmd == "show" and AtMaxLevel() then
            Print("you're at max level, so there's no XP to track.")
        end
    elseif cmd == "center" then
        CenterCard()
    else
        Print("type /xpc to open the settings. Shortcuts:")
        print("  /xpc pause, /xpc resume")
        print("  /xpc reset - start a new session")
        print("  /xpc lock, /xpc unlock")
        print("  /xpc scale 0.5-2")
        print("  /xpc hide, /xpc show")
        print("  /xpc center")
        return
    end
    ApplySettings()
    ns.RefreshSettings()
end

---------------------------------------------------------------------------
-- Events
---------------------------------------------------------------------------

frame:RegisterEvent("ADDON_LOADED")
frame:RegisterEvent("PLAYER_ENTERING_WORLD")
frame:RegisterEvent("PLAYER_XP_UPDATE")
frame:RegisterEvent("PLAYER_LEVEL_UP")
frame:RegisterEvent("UPDATE_EXHAUSTION")
frame:RegisterEvent("PLAYER_REGEN_DISABLED")
frame:RegisterEvent("PLAYER_REGEN_ENABLED")
frame:RegisterEvent("PLAYER_LOGOUT")
frame:RegisterEvent("QUEST_LOG_UPDATE")
frame:RegisterEvent("QUEST_TURNED_IN")

frame:SetScript("OnEvent", function(self, event, ...)
    if event == "ADDON_LOADED" then
        if ... ~= ADDON_NAME then return end
        self:UnregisterEvent("ADDON_LOADED")
        XPCounterDB = XPCounterDB or {}
        XPCounterCharDB = XPCounterCharDB or {}
        db, charDB = XPCounterDB, XPCounterCharDB
        for k, v in pairs(DEFAULTS) do
            if db[k] == nil then db[k] = v end
        end
        ns.db = db
        frame:SetScale(db.scale)
        ApplyPosition()
        ApplyLook()
        Layout()
        SetupGameBar()
        if GameBarManager() then ApplyGameBar() end   -- hidden from the start, no flash at login
        -- Adds the page under Esc > Options > AddOns. If the game's settings API ever
        -- changes, show the error but keep the card working.
        local ok, err = pcall(ns.RegisterOptions)
        if not ok then geterrorhandler()(err) end

    elseif event == "PLAYER_ENTERING_WORLD" then
        local isInitialLogin, isReloadingUi = ...
        inCombat = InCombatLockdown() and true or false
        if isInitialLogin or isReloadingUi or not ticker then
            StartSession()
            -- PLAYER_LOGOUT also fires on /reload, so a reload picks the session back up.
            local saved = isReloadingUi and charDB.session
            if saved then
                session.gained = saved.gained or 0
                session.active = saved.active or 0
                session.levels = saved.levels or 0
                session.paused = saved.paused and true or false
            end
            charDB.session = nil
        end
        if not ticker then
            ticker = C_Timer.NewTicker(TICK, Tick)
        end
        ScanQuests()
        Update()
        SetupGameBar()
        ApplyGameBar()

    elseif event == "PLAYER_XP_UPDATE" then
        TrackXP()
        Update()

    elseif event == "PLAYER_LEVEL_UP" then
        if not session.paused then
            session.levels = session.levels + 1
        end
        ScanQuests()   -- quest rewards are worked out for your level
        Update()

    elseif event == "QUEST_LOG_UPDATE" or event == "QUEST_TURNED_IN" then
        ScanQuests()
        Update()

    elseif event == "UPDATE_EXHAUSTION" then
        Update()

    elseif event == "PLAYER_REGEN_DISABLED" or event == "PLAYER_REGEN_ENABLED" then
        inCombat = (event == "PLAYER_REGEN_DISABLED")
        Update()

    elseif event == "PLAYER_LOGOUT" then
        charDB.session = { gained = session.gained, active = session.active,
                           levels = session.levels, paused = session.paused }
    end
end)
