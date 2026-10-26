"""Stock levels for the warehouse's items."""

# An item is flagged for reorder once its stock falls to this many units or
# fewer.
REORDER_THRESHOLD = 37


def needs_reorder(on_hand: int) -> bool:
    """Whether an item with this many units on hand should be reordered."""
    return on_hand <= REORDER_THRESHOLD
