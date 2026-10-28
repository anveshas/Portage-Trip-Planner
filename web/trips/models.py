from django.db import models


class Trip(models.Model):
    origin = models.CharField(max_length=8)
    destination = models.CharField(max_length=8)
    start_date = models.DateField()
    end_date = models.DateField()
    travelers = models.PositiveSmallIntegerField()
    budget = models.PositiveIntegerField()
    pace = models.CharField(max_length=16)
    interests = models.JSONField(default=list)
    stops = models.JSONField(default=list)
    created_at = models.DateTimeField(auto_now_add=True)

    def __str__(self):
        return f"{self.origin} to {self.destination}"


class Itinerary(models.Model):
    trip = models.OneToOneField(Trip, on_delete=models.CASCADE, related_name="itinerary")
    plan = models.JSONField()
    narrative = models.TextField()
    narrative_source = models.CharField(max_length=32)
    created_at = models.DateTimeField(auto_now_add=True)
