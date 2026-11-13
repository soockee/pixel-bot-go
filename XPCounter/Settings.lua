-- XP Counter: settings.
-- A small window that opens beside the card (right-click it, or type /xpc),
-- and the same controls on a page under Esc > Options > AddOns.

local _, ns = ...
local C = ns.C

local PANEL_WIDTH = 270
local LEFT        = 16                       -- content inset
local INNER       = PANEL_WIDTH - 2 * LEFT   -- content width
local COL2        = LEFT + 124               -- second checkbox column

local hosts = {}   -- one refresh function per built copy of the controls

local function RefreshAll()
    for _, refresh in ipairs(hosts) do refresh() end
end
ns.RefreshSettings = RefreshAll

local function Changed()
    ns.ApplySettings()
    RefreshAll()
end

---------------------------------------------------------------------------
-- Widgets, drawn from plain textures so they match the card
---------------------------------------------------------------------------

local function NewSection(parent, y, text)
    local fs = ns.NewText(parent, 10, C.hint)
    fs:SetPoint("TOPLEFT", LEFT, -y)
    fs:SetText(text)
end

local function NewCheckbox(parent, x, y, text, get, set)
    local b = CreateFrame("Button", nil, parent)
    b:SetPoint("TOPLEFT", x, -y)
    b:SetSize(118, 16)

    local border = b:CreateTexture(nil, "BACKGROUND")
    border:SetSize(12, 12)
    border:SetPoint("LEFT")
    border:SetColorTexture(1, 1, 1, 0.25)

    local inner = b:CreateTexture(nil, "BORDER")
    inner:SetSize(10, 10)
    inner:SetPoint("CENTER", border)
    inner:SetColorTexture(0.06, 0.06, 0.08, 1)

    local fill = b:CreateTexture(nil, "ARTWORK")
    fill:SetSize(8, 8)
    fill:SetPoint("CENTER", border)

    local label = ns.NewText(b, 11, C.value)
    label:SetPoint("LEFT", border, "RIGHT", 7, 0)
    label:SetText(text)
    b:SetWidth(math.max(118, 19 + math.ceil(label:GetStringWidth())))   -- the label is clickable too

    b:SetScript("OnClick", function()
        set(not get())
        Changed()
    end)
    b:SetScript("OnEnter", function() border:SetColorTexture(1, 1, 1, 0.5) end)
    b:SetScript("OnLeave", function() border:SetColorTexture(1, 1, 1, 0.25) end)

    return function()
        local a = ns.Accent()
        fill:SetColorTexture(a.r, a.g, a.b, 1)
        fill:SetShown(get() and true or false)
    end
end

local function Percent(v)
    return string.format("%d%%", math.floor(v * 100 + 0.5))
end

local function NewSlider(parent, y, text, min, max, step, get, set)
    local label = ns.NewText(parent, 11, C.value)
    label:SetPoint("TOPLEFT", LEFT, -y)
    label:SetText(text)

    local valueText = ns.NewText(parent, 11, C.label, "RIGHT")
    valueText:SetPoint("TOPRIGHT", parent, "TOPLEFT", LEFT + INNER, -y)

    local slider = CreateFrame("Slider", nil, parent)
    slider:SetOrientation("HORIZONTAL")
    slider:SetPoint("TOPLEFT", LEFT, -(y + 16))
    slider:SetSize(INNER, 12)
    slider:SetMinMaxValues(min, max)
    slider:SetValueStep(step)
    slider:SetObeyStepOnDrag(true)
    slider:EnableMouse(true)
    slider:EnableMouseWheel(true)

    local track = slider:CreateTexture(nil, "BACKGROUND")
    track:SetPoint("LEFT")
    track:SetPoint("RIGHT")
    track:SetHeight(2)
    track:SetColorTexture(1, 1, 1, 0.15)

    slider:SetThumbTexture(ns.WHITE)
    local thumb = slider:GetThumbTexture()
    thumb:SetSize(6, 12)

    local fill = slider:CreateTexture(nil, "ARTWORK")
    fill:SetPoint("LEFT")
    fill:SetPoint("RIGHT", thumb, "CENTER")
    fill:SetHeight(2)

    slider:SetScript("OnValueChanged", function(_, value)
        value = math.floor(value * 100 + 0.5) / 100
        valueText:SetText(Percent(value))
        if value ~= get() then
            set(value)
            Changed()
        end
    end)
    slider:SetScript("OnMouseWheel", function(self, delta)
        self:SetValue(self:GetValue() + delta * step)
    end)

    return function()
        local a = ns.Accent()
        thumb:SetVertexColor(a.r, a.g, a.b)
        fill:SetColorTexture(a.r, a.g, a.b, 0.9)
        slider:SetValue(get())
        valueText:SetText(Percent(get()))
    end
end

local function NewSwatches(parent, y)
    local label = ns.NewText(parent, 11, C.value)
    label:SetPoint("TOPLEFT", LEFT, -(y + 3))
    label:SetText("Color")

    local rings = {}
    for i, accent in ipairs(ns.ACCENTS) do
        local b = CreateFrame("Button", nil, parent)
        b:SetSize(18, 18)
        b:SetPoint("TOPLEFT", LEFT + 64 + (i - 1) * 26, -y)

        local ring = b:CreateTexture(nil, "BACKGROUND")
        ring:SetAllPoints()
        ring:SetColorTexture(1, 1, 1, 0.9)

        local chip = b:CreateTexture(nil, "ARTWORK")
        chip:SetPoint("TOPLEFT", 2, -2)
        chip:SetPoint("BOTTOMRIGHT", -2, 2)
        chip:SetColorTexture(accent.r, accent.g, accent.b, 1)

        b:SetScript("OnClick", function()
            ns.db.accent = accent.key
            Changed()
        end)
        b:SetScript("OnEnter", function(self)
            GameTooltip:SetOwner(self, "ANCHOR_TOP")
            GameTooltip:AddLine(accent.name, 1, 1, 1)
            GameTooltip:Show()
        end)
        b:SetScript("OnLeave", function() GameTooltip:Hide() end)
        rings[accent.key] = ring
    end

    return function()
        local current = ns.Accent().key
        for key, ring in pairs(rings) do ring:SetShown(key == current) end
    end
end

-- `text` can also be a function, for a label that changes; it's re-read on every refresh.
local function NewButton(parent, x, y, width, height, text, onClick)
    local b = CreateFrame("Button", nil, parent)
    b:SetPoint("TOPLEFT", x, -y)
    b:SetSize(width, height)

    local bg = b:CreateTexture(nil, "BACKGROUND")
    bg:SetAllPoints()
    bg:SetColorTexture(1, 1, 1, 0.07)

    local label = ns.NewText(b, 11, C.value, "CENTER")
    label:SetPoint("CENTER")

    local function refresh()
        label:SetText(type(text) == "function" and text() or text)
    end
    refresh()

    b:SetScript("OnClick", function() onClick() end)
    b:SetScript("OnEnter", function() bg:SetColorTexture(1, 1, 1, 0.14) end)
    b:SetScript("OnLeave", function() bg:SetColorTexture(1, 1, 1, 0.07) end)
    return refresh
end

---------------------------------------------------------------------------
-- The controls (built once for the window and once for the options page)
---------------------------------------------------------------------------

local function Flag(key, invert)
    local function get()
        if invert then return not ns.db[key] end
        return ns.db[key]
    end
    local function set(value)
        if invert then value = not value end
        ns.db[key] = value
    end
    return get, set
end

local function Number(key)
    return function() return ns.db[key] end, function(value) ns.db[key] = value end
end

-- Builds the controls inside `host`, starting `top` pixels down.
-- Returns their refresh function and the y position below the last one.
local function BuildControls(host, top)
    local parts = {}
    local function add(refresh) parts[#parts + 1] = refresh end
    local y = top

    NewSection(host, y, "CARD"); y = y + 18
    add(NewCheckbox(host, LEFT, y, "Show the XP card", Flag("hidden", true))); y = y + 20
    add(NewCheckbox(host, LEFT, y, "Lock position", Flag("locked")))
    NewButton(host, COL2, y - 1, LEFT + INNER - COL2, 18, "Center card", function()
        ns.CenterCard()
        Changed()
    end)
    y = y + 20
    add(NewCheckbox(host, LEFT, y, "Hide in combat", Flag("hideInCombat"))); y = y + 28

    NewSection(host, y, "SHOW"); y = y + 18
    add(NewCheckbox(host, LEFT, y, "XP", Flag("showXP")))
    add(NewCheckbox(host, COL2, y, "To level", Flag("showLeft"))); y = y + 20
    add(NewCheckbox(host, LEFT, y, "XP / hour", Flag("showRate")))
    add(NewCheckbox(host, COL2, y, "Time to level", Flag("showEta"))); y = y + 20
    add(NewCheckbox(host, LEFT, y, "Session timer", Flag("showTimer")))
    add(NewCheckbox(host, COL2, y, "Quests ready", Flag("showQuests"))); y = y + 20
    add(NewCheckbox(host, LEFT, y, "XP bar", Flag("showBar")))
    add(NewCheckbox(host, COL2, y, "Rested XP", Flag("showRested"))); y = y + 28

    NewSection(host, y, "LOOK"); y = y + 18
    add(NewSlider(host, y, "Scale", 0.5, 2, 0.05, Number("scale"))); y = y + 38
    add(NewSlider(host, y, "Background", 0, 1, 0.05, Number("alpha"))); y = y + 38
    add(NewSwatches(host, y)); y = y + 30

    NewSection(host, y, "GAME UI"); y = y + 18
    add(NewCheckbox(host, LEFT, y, "Hide the game's XP bar", Flag("hideGameBar"))); y = y + 28

    NewSection(host, y, "SESSION"); y = y + 18
    local half = (INNER - 8) / 2
    add(NewButton(host, LEFT, y, half, 22, function()
        return ns.IsPaused() and "Resume session" or "Pause session"
    end, ns.TogglePause))
    NewButton(host, LEFT + half + 8, y, half, 22, "Reset session", ns.ResetSession)
    y = y + 22

    local function refresh()
        for _, part in ipairs(parts) do part() end
    end
    hosts[#hosts + 1] = refresh
    return refresh, y
end

---------------------------------------------------------------------------
-- The window
---------------------------------------------------------------------------

local panel = CreateFrame("Frame", "XPCounterSettings", UIParent, "BackdropTemplate")
panel:SetWidth(PANEL_WIDTH)
panel:SetFrameStrata("DIALOG")
panel:SetClampedToScreen(true)
panel:SetMovable(true)
panel:EnableMouse(true)
panel:RegisterForDrag("LeftButton")
panel:SetBackdrop({ bgFile = ns.WHITE, edgeFile = ns.WHITE, edgeSize = 1 })
panel:SetBackdropColor(0.05, 0.05, 0.07, 0.96)
panel:SetBackdropBorderColor(1, 1, 1, 0.1)
panel:Hide()
panel:SetScript("OnDragStart", panel.StartMoving)
panel:SetScript("OnDragStop", panel.StopMovingOrSizing)
table.insert(UISpecialFrames, "XPCounterSettings")   -- Esc closes it

local title = ns.NewText(panel, 13, C.value)
title:SetPoint("TOPLEFT", LEFT, -14)
title:SetText("XP Counter")

local close = CreateFrame("Button", nil, panel)
close:SetSize(20, 20)
close:SetPoint("TOPRIGHT", -8, -8)
local strokes = {}
for i, angle in ipairs({ 45, -45 }) do
    local stroke = close:CreateTexture(nil, "ARTWORK")
    stroke:SetSize(12, 1.5)
    stroke:SetPoint("CENTER")
    stroke:SetColorTexture(1, 1, 1, 0.55)
    stroke:SetRotation(math.rad(angle))
    strokes[i] = stroke
end
close:SetScript("OnClick", function() panel:Hide() end)
close:SetScript("OnEnter", function()
    for _, stroke in ipairs(strokes) do stroke:SetColorTexture(1, 1, 1, 1) end
end)
close:SetScript("OnLeave", function()
    for _, stroke in ipairs(strokes) do stroke:SetColorTexture(1, 1, 1, 0.55) end
end)

local refreshPanel, bottom = BuildControls(panel, 44)
panel:SetHeight(bottom + 16)
panel:SetScript("OnShow", refreshPanel)

-- Opens beside the card, on whichever side has more room, or centered if the card isn't showing.
local function PlacePanel()
    panel:ClearAllPoints()
    local card = ns.card
    local left, bottomEdge, width, height = card:GetRect()
    if not (card:IsVisible() and left) then
        panel:SetPoint("CENTER", UIParent, "CENTER", 0, 0)
        return
    end
    local s = card:GetEffectiveScale() / panel:GetEffectiveScale()
    left, bottomEdge, width, height = left * s, bottomEdge * s, width * s, height * s
    local top = bottomEdge + height
    if left + width / 2 < UIParent:GetWidth() / 2 then
        panel:SetPoint("TOPLEFT", UIParent, "BOTTOMLEFT", left + width + 12, top)
    else
        panel:SetPoint("TOPRIGHT", UIParent, "BOTTOMLEFT", left - 12, top)
    end
end

function ns.ToggleSettings()
    if panel:IsShown() then
        panel:Hide()
    else
        PlacePanel()
        panel:Show()
    end
end

---------------------------------------------------------------------------
-- The page under Esc > Options > AddOns
---------------------------------------------------------------------------

function ns.RegisterOptions()
    if not (Settings and Settings.RegisterCanvasLayoutCategory and Settings.RegisterAddOnCategory) then
        return
    end
    local page = CreateFrame("Frame")
    page:Hide()

    local heading = ns.NewText(page, 16, C.value)
    heading:SetPoint("TOPLEFT", LEFT, -16)
    heading:SetText("XP Counter")

    local tip = ns.NewText(page, 11, C.label)
    tip:SetPoint("TOPLEFT", LEFT, -40)
    tip:SetText("Tip: right-click the card to open these settings next to it.")

    local refresh = BuildControls(page, 70)
    page:SetScript("OnShow", refresh)

    local category = Settings.RegisterCanvasLayoutCategory(page, "XP Counter")
    Settings.RegisterAddOnCategory(category)
end
