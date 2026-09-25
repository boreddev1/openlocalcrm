import React, { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { apiFetch } from '../api/client';
import { MapContainer, TileLayer, Marker, Popup } from 'react-leaflet';
import { MapPin, Phone, AlertCircle, ChevronDown, ChevronUp, MapPinOff } from 'lucide-react';
import { Link } from 'react-router-dom';
import L from 'leaflet';

// Fix Leaflet default marker icons for Vite
delete (L.Icon.Default.prototype as any)._getIconUrl;
L.Icon.Default.mergeOptions({
  iconRetinaUrl: 'https://unpkg.com/leaflet@1.9.4/dist/images/marker-icon-2x.png',
  iconUrl: 'https://unpkg.com/leaflet@1.9.4/dist/images/marker-icon.png',
  shadowUrl: 'https://unpkg.com/leaflet@1.9.4/dist/images/marker-shadow.png',
});

export const MapViewPage: React.FC = () => {
  const [showUnmapped, setShowUnmapped] = useState(false);

  const { data: rawContacts = [], isLoading } = useQuery<any[]>({
    queryKey: ['contacts'],
    queryFn: () => apiFetch('/api/v1/contacts'),
  });
  const contacts = Array.isArray(rawContacts) ? rawContacts : [];

  const mappedContacts = contacts.filter(
    (c) =>
      typeof c.latitude === 'number' &&
      typeof c.longitude === 'number' &&
      !isNaN(c.latitude) &&
      !isNaN(c.longitude),
  );

  const unmappedContacts = contacts.filter(
    (c) =>
      !(
        typeof c.latitude === 'number' &&
        typeof c.longitude === 'number' &&
        !isNaN(c.latitude) &&
        !isNaN(c.longitude)
      ),
  );

  // Default center: Germany overview
  const defaultCenter: [number, number] =
    mappedContacts.length === 1
      ? [mappedContacts[0].latitude, mappedContacts[0].longitude]
      : [51.1657, 10.4515];
  const defaultZoom = mappedContacts.length === 1 ? 13 : 6;

  return (
    <div className="space-y-4 max-w-7xl mx-auto flex flex-col h-[calc(100vh-6rem)]">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 shrink-0">
        <div>
          <h1 className="text-2xl font-bold text-slate-100 flex items-center gap-2.5">
            <MapPin className="w-6 h-6 text-emerald-400" />
            D2D & Außendienst-Gebietskarte
          </h1>
          <p className="text-sm text-slate-400">
            {isLoading
              ? 'Lade Kontakte...'
              : `${mappedContacts.length} von ${contacts.length} Kontakten mit Geokoordinaten auf der Karte verortet`}
          </p>
        </div>

        {unmappedContacts.length > 0 && (
          <button
            type="button"
            onClick={() => setShowUnmapped(!showUnmapped)}
            className="inline-flex items-center gap-2 px-3.5 py-2 bg-slate-900 hover:bg-slate-800 text-amber-300 text-xs font-semibold rounded-xl border border-amber-500/30 transition-colors cursor-pointer self-start sm:self-auto shadow-sm"
          >
            <AlertCircle className="w-4 h-4 text-amber-400" />
            <span>Kontakte ohne Standort ({unmappedContacts.length})</span>
            {showUnmapped ? (
              <ChevronUp className="w-3.5 h-3.5" />
            ) : (
              <ChevronDown className="w-3.5 h-3.5" />
            )}
          </button>
        )}
      </div>

      {/* Unmapped Contacts Drawer */}
      {showUnmapped && unmappedContacts.length > 0 && (
        <div className="bg-slate-900 border border-amber-500/30 rounded-2xl p-4 shrink-0 shadow-lg space-y-3">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2 text-xs text-amber-300 font-semibold">
              <MapPinOff className="w-4 h-4 text-amber-400" />
              <span>
                Kontakte ohne Geokoordinaten ({unmappedContacts.length}) — Beim Speichern einer
                Adresse mit 5-stelliger PLZ wird der Standort automatisch ermittelt.
              </span>
            </div>
            <button
              onClick={() => setShowUnmapped(false)}
              className="text-xs text-slate-400 hover:text-slate-200"
            >
              Schließen
            </button>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-2.5 max-h-48 overflow-y-auto pr-1">
            {unmappedContacts.map((c) => (
              <div
                key={c.id}
                className="p-2.5 bg-slate-950 border border-slate-800 rounded-xl text-xs space-y-1"
              >
                <div className="font-semibold text-slate-200">
                  {c.first_name} {c.last_name}
                </div>
                <div className="text-[11px] text-slate-400">
                  {c.address_zip || c.address_city ? (
                    `${c.address_street ? c.address_street + ', ' : ''}${c.address_zip || ''} ${c.address_city || ''}`.trim()
                  ) : (
                    <span className="text-slate-500 italic">Keine Adresse hinterlegt</span>
                  )}
                </div>
                <div className="pt-1">
                  <Link
                    to="/contacts"
                    className="text-[11px] text-emerald-400 hover:text-emerald-300 font-medium"
                  >
                    Adresse bearbeiten &rarr;
                  </Link>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Map Card */}
      <div className="flex-1 bg-slate-900 border border-slate-800 rounded-2xl overflow-hidden shadow-2xl relative">
        <MapContainer
          center={defaultCenter}
          zoom={defaultZoom}
          scrollWheelZoom={true}
          className="w-full h-full min-h-[400px] z-10"
        >
          <TileLayer
            attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
            url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
          />

          {mappedContacts.map((contact) => (
            <Marker key={contact.id} position={[contact.latitude, contact.longitude]}>
              <Popup className="custom-popup">
                <div className="p-1 space-y-1 text-slate-900">
                  <div className="font-bold text-sm">
                    {contact.first_name} {contact.last_name}
                  </div>
                  {contact.position && (
                    <div className="text-xs text-slate-600">{contact.position}</div>
                  )}
                  {contact.address_street && (
                    <div className="text-xs flex items-center gap-1 text-slate-700">
                      <MapPin className="w-3 h-3 text-emerald-600" /> {contact.address_street},{' '}
                      {contact.address_city}
                    </div>
                  )}
                  {contact.phone && (
                    <div className="text-xs flex items-center gap-1 text-slate-700">
                      <Phone className="w-3 h-3 text-emerald-600" /> {contact.phone}
                    </div>
                  )}
                </div>
              </Popup>
            </Marker>
          ))}
        </MapContainer>
      </div>
    </div>
  );
};
