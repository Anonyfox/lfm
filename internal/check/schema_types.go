package check

import "strings"

// Schema.org descendants used by the offline identity and sitemap checks.
// Type names from https://schema.org/docs/full.html (Schema.org contributors, CC BY-SA 3.0).
var organizationTypes = schemaTypes(`AccountingService AdultEntertainment Airline AmusementPark AnimalShelter
ArchiveOrganization ArtGallery Attorney Audiology AutoBodyShop AutoDealer AutoPartsStore
AutoRental AutoRepair AutoWash AutomatedTeller AutomotiveBusiness Bakery BankOrCreditUnion
BarOrPub BeautySalon BedAndBreakfast BikeStore BookStore BowlingAlley Brewery
CafeOrCoffeeShop Campground Casino ChildCare ClothingStore CollegeOrUniversity ComedyClub
CommunityHealth ComputerStore Consortium ConvenienceStore Cooperative Corporation
CovidTestingFacility DanceGroup DaySpa Dentist DepartmentStore Dermatology DiagnosticLab
DietNutrition Distillery DryCleaningOrLaundry EducationalOrganization Electrician
ElectronicsStore ElementarySchool Emergency EmergencyService EmploymentAgency
EntertainmentBusiness ExerciseGym FastFoodRestaurant FinancialService FireStation Florist
FoodEstablishment FundingAgency FundingScheme FurnitureStore GardenStore GasStation
GeneralContractor Geriatric GolfCourse GovernmentOffice GovernmentOrganization
GroceryStore Gynecologic HVACBusiness HairSalon HardwareStore HealthAndBeautyBusiness
HealthClub HighSchool HobbyShop HomeAndConstructionBusiness HomeGoodsStore Hospital Hostel
Hotel HousePainter IceCreamShop IndividualPhysician InsuranceAgency InternetCafe
JewelryStore LegalService Library LibrarySystem LiquorStore LocalBusiness Locksmith
LodgingBusiness MedicalBusiness MedicalClinic MedicalOrganization MensClothingStore
MiddleSchool Midwifery MobilePhoneStore Motel MotorcycleDealer MotorcycleRepair
MovieRentalStore MovieTheater MovingCompany MusicGroup MusicStore NGO NailSalon
NewsMediaOrganization NightClub Notary Nursing Obstetric OfficeEquipmentStore Oncologic
OnlineBusiness OnlineMarketplace OnlineStore Ophthalmology Optician Optometric
Organization Otolaryngologic OutletStore PawnShop Pediatric PerformingGroup PetStore
Pharmacy Physician PhysiciansOffice Physiotherapy PlasticSurgery Plumber Podiatric
PoliceStation PoliticalParty PostOffice Preschool PrimaryCare ProfessionalService Project
Psychiatric PublicHealth PublicSwimmingPool RadioStation RealEstateAgent RecyclingCenter
ResearchOrganization ResearchProject Resort Restaurant RoofingContractor School
SearchRescueOrganization SelfStorage ShoeStore ShoppingCenter SkiResort SportingGoodsStore
SportsActivityLocation SportsClub SportsOrganization SportsTeam StadiumOrArena Store
TattooParlor TelevisionStation TennisComplex TheaterGroup TireShop
TouristInformationCenter ToyStore TravelAgency VacationRental VeterinaryCare
WholesaleStore Winery WorkersUnion`)
var webPageTypes = schemaTypes(`AboutPage CheckoutPage CollectionPage ContactPage FAQPage ImageGallery ItemPage
MediaGallery MedicalWebPage ProfilePage QAPage RealEstateListing SearchResultsPage
VideoGallery WebPage Article BlogPosting`)

func schemaTypes(names string) map[string]bool {
	types := map[string]bool{}
	for name := range strings.FieldsSeq(names) {
		types["https://schema.org/"+name] = true
		types["http://schema.org/"+name] = true
	}
	return types
}

func hasSchemaType(node map[string]any, types map[string]bool) bool {
	kinds, _ := node["@type"].([]any)
	for _, kind := range kinds {
		name, _ := kind.(string)
		if types[name] {
			return true
		}
	}
	return false
}
