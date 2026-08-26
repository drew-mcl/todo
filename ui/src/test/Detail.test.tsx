import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Detail } from "../components/Detail";
import { api, type Task } from "../api";

const task: Task = {
  id: 1,
  topic: "prod issue",
  title: "chase the vendor",
  note: "they missed two dates",
  due: "2026-09-16",
  // What the list shows. Deliberately not something the date reader can read.
  dueLabel: "Wed 16 Sep",
  overdue: false,
  assignee: "sam",
  priority: 2,
  done: false,
  tags: ["board"],
  raw: "prod issue | chase the vendor | 16/9 @sam !!",
  batchId: 3,
  capturedAt: "2026-08-25",
  capturedWhen: "Today",
};

beforeEach(() => vi.restoreAllMocks());

describe("the detail panel", () => {
  // The regression that made every overdue or far-off task unsaveable: the due
  // field is seeded with a display label, and sending it back was a 422.
  it("does not send the due date when it was not edited", async () => {
    const update = vi.spyOn(api, "update").mockResolvedValue(task);
    render(<Detail task={task} onClose={() => {}} onSaved={() => {}} onDeleted={() => {}} />);

    await userEvent.click(screen.getByRole("button", { name: "save" }));

    await waitFor(() => expect(update).toHaveBeenCalled());
    const [, patch] = update.mock.calls[0];
    expect(patch).not.toHaveProperty("due");
    expect(patch).toMatchObject({ title: "chase the vendor", assignee: "sam" });
  });

  it("sends the due date once it has been typed in", async () => {
    const update = vi.spyOn(api, "update").mockResolvedValue(task);
    render(<Detail task={task} onClose={() => {}} onSaved={() => {}} onDeleted={() => {}} />);

    const due = screen.getByPlaceholderText("today, eow, 25/12");
    await userEvent.clear(due);
    await userEvent.type(due, "eow");
    await userEvent.click(screen.getByRole("button", { name: "save" }));

    await waitFor(() => expect(update).toHaveBeenCalled());
    expect(update.mock.calls[0][1]).toMatchObject({ due: "eow" });
  });

  it("keeps your edits on screen when the server refuses one field", async () => {
    const { ApiError } = await import("../api");
    vi.spyOn(api, "update").mockRejectedValue(
      new ApiError(422, '"wednesbury" is not a date I understand.', "due"),
    );
    const onClose = vi.fn();
    render(<Detail task={task} onClose={onClose} onSaved={() => {}} onDeleted={() => {}} />);

    const title = screen.getByDisplayValue("chase the vendor");
    await userEvent.clear(title);
    await userEvent.type(title, "renamed");
    await userEvent.click(screen.getByRole("button", { name: "save" }));

    expect(await screen.findByText(/not a date I understand/)).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
    // The work is still there to correct, not thrown away with the error.
    expect(screen.getByDisplayValue("renamed")).toBeInTheDocument();
  });

  it("shows where the task came from", () => {
    render(<Detail task={task} onClose={() => {}} onSaved={() => {}} onDeleted={() => {}} />);
    expect(screen.getByText(/captured today/i)).toBeInTheDocument();
    expect(screen.getByText(task.raw)).toBeInTheDocument();
  });
});
