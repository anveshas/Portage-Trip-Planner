from django.contrib import admin

from trips.models import Itinerary, Trip


class ItineraryInline(admin.StackedInline):
    model = Itinerary
    extra = 0


@admin.register(Trip)
class TripAdmin(admin.ModelAdmin):
    list_display = ("origin", "destination", "start_date", "budget", "created_at")
    inlines = [ItineraryInline]
