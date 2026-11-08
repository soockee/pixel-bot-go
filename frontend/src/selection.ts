// Selection overlay: a transparent window framed by the addon's brackets. Its
// own bounds become the capture area, so moving and resizing it is the input.
import { Service } from "./api";
import { $ } from "./dom";

const size = $("size");
const showSize = () => {
  const r = window.devicePixelRatio || 1;
  size.textContent = `${Math.round(innerWidth * r)} × ${Math.round(innerHeight * r)} px`;
};
addEventListener("resize", showSize);
showSize();

$("confirm").addEventListener("click", () => void Service.ConfirmSelection());
$("cancel").addEventListener("click", () => void Service.CloseSelection());
$("clear").addEventListener("click", () => {
  void Service.ClearSelection();
  void Service.CloseSelection();
});
addEventListener("keydown", (e) => {
  if (e.key === "Enter") void Service.ConfirmSelection();
  if (e.key === "Escape") void Service.CloseSelection();
});
