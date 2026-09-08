"""Shared field identity and widget-page lookup for the PDF form helpers."""
from __future__ import annotations


def field_locations(reader):
    if not hasattr(reader, "get_pages_showing_field"):
        raise ValueError("PDF forms require pypdf >= 6.10; upgrade the task environment")
    fields = reader.get_fields() or {}
    locations = {}
    for field_id, field in fields.items():
        if field is None or not field.get("/FT"):
            continue
        pages = set()
        for page in reader.get_pages_showing_field(field):
            number = page.page_number
            if number is None or not 0 <= number < len(reader.pages):
                raise ValueError(f"cannot locate widget page for field {field_id!r}")
            pages.add(number + 1)
        locations[field_id] = (field, sorted(pages))
    return locations
