import React from 'react';
import { useQuery } from '@tanstack/react-query';
import { apiFetch } from '../api/client';
import { MapContainer, TileLayer, Marker, Popup } from 'react-leaflet';
import { MapPin, Phone } from 'lucide-react';
import L from 'leaflet';

// Fix Leaflet default marker icons for Vite
delete (L.Icon.Default.prototype as any)._getIconUrl;
L.Icon.Default.mergeOptions({
  iconRetinaUrl: 'https://unpkg.com/leaflet@1.9.4/dist/images/marker-icon-2x.png',
  iconUrl: 'https://unpkg.com/leaflet@1.9.4/dist/images/marker-icon.png',
  shadowUrl: 'https://unpkg.com/leaflet@1.9.4/dist/images/marker-shadow.png',
});

export const MapViewPage: React.FC = () => {
  const { data: rawContacts = [] } = useQuery<any[]>({
    queryKey: ['contacts'],
    queryFn: () => apiFetch('/api/v1/contacts'),
  });
  const contacts = Array.isArray(rawContacts) ? rawContacts : [];

  // Default center: Frankfurt am Main
  const defaultCenter: [number, number] = [50.1109, 8.6821];

  return (
    <div className="space-y-4 max-w-7xl mx-auto flex flex-col h-[calc(100vh-6rem)]">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 shrink-0">
        <div>
          <h1 className="text-2xl font-bold text-slate-100">D2D & Außendienst-Gebietskarte</h1>
          <p className="text-sm text-slate-400">
            Übersicht aller Kunden, Adressen und Leads vor Ort
          </p>
        </div>
      </div>

      {/* Map Card */}
      <div className="flex-1 bg-slate-900 border border-slate-800 rounded-2xl overflow-hidden shadow-2xl relative">
        <MapContainer
          center={defaultCenter}
          zoom={12}
          scrollWheelZoom={true}
          className="w-full h-full min-h-[400px] z-10"
        >
          <TileLayer
            attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
            url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
          />

          {contacts.map((contact) => {
            const lat = contact.latitude || 50.1109 + (Math.random() - 0.5) * 0.08;
            const lng = contact.longitude || 8.6821 + (Math.random() - 0.5) * 0.08;

            return (
              <Marker key={contact.id} position={[lat, lng]}>
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
            );
          })}
        </MapContainer>
      </div>
    </div>
  );
};
